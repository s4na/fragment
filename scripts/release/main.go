// Release packaging uses only the Go standard library. All generated files
// stay under the checkout's ignored bin/release directory.
package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

var versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type version [3]uint64

func parseVersion(s string) (version, error) {
	match := versionPattern.FindStringSubmatch(s)
	if match == nil {
		return version{}, fmt.Errorf("invalid release version: %q", s)
	}
	var result version
	for i := range result {
		n, err := strconv.ParseUint(match[i+1], 10, 64)
		if err != nil {
			return version{}, err
		}
		result[i] = n
	}
	return result, nil
}

func (v version) String() string { return fmt.Sprintf("v%d.%d.%d", v[0], v[1], v[2]) }

func latestVersion(reader io.Reader) (version, error) {
	var latest version
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		candidate, err := parseVersion(scanner.Text())
		if err != nil {
			continue // unrelated and prerelease tags are not patch releases
		}
		for i := range candidate {
			if candidate[i] > latest[i] {
				latest = candidate
				break
			}
			if candidate[i] < latest[i] {
				break
			}
		}
	}
	return latest, scanner.Err()
}

type target struct{ OS, Arch, SHA string }

var targets = []target{{OS: "darwin", Arch: "arm64"}, {OS: "darwin", Arch: "amd64"}, {OS: "linux", Arch: "arm64"}, {OS: "linux", Arch: "amd64"}}

func archiveName(tag string, target target) string {
	return fmt.Sprintf("fragment_%s_%s_%s.tar.gz", strings.TrimPrefix(tag, "v"), target.OS, target.Arch)
}

func archive(binary, destination string) error {
	input, err := os.Open(binary)
	if err != nil {
		return err
	}
	defer input.Close()
	stat, err := input.Stat()
	if err != nil {
		return err
	}
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer output.Close()
	gz := gzip.NewWriter(output)
	tw := tar.NewWriter(gz)
	// Fixed metadata makes re-packaging identical bytes deterministic.
	err = tw.WriteHeader(&tar.Header{Name: "fragment", Mode: 0o755, Size: stat.Size(), Typeflag: tar.TypeReg})
	if err == nil {
		_, err = io.Copy(tw, input)
	}
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := gz.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return output.Close()
}

func build(tag, commit, output string) error {
	for _, target := range targets {
		binary := filepath.Join(output, "fragment_"+target.OS+"_"+target.Arch)
		flags := "-X main.buildVersion=" + strings.TrimPrefix(tag, "v") + " -X main.buildCommit=" + commit + " -X main.buildSource=git:" + commit
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", flags, "-o", binary, "./cmd/fragment")
		for _, env := range os.Environ() {
			key, _, _ := strings.Cut(env, "=")
			switch key {
			case "GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS", "GOWORK":
				continue
			}
			cmd.Env = append(cmd.Env, env)
		}
		cmd.Env = append(cmd.Env, "CGO_ENABLED=0", "GOFLAGS=", "GOWORK=off", "GOOS="+target.OS, "GOARCH="+target.Arch)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s/%s: %w", target.OS, target.Arch, err)
		}
		if err := archive(binary, filepath.Join(output, archiveName(tag, target))); err != nil {
			return err
		}
		if err := os.Remove(binary); err != nil {
			return err
		}
	}
	return manifest(tag, output)
}

const formulaTemplate = `# Generated after a PR merge. Do not edit release URLs or checksums manually.
class Fragment < Formula
  desc "Local Markdown documents shared by CLI, MCP, and Web UI"
  homepage "https://github.com/s4na/fragment"
  version "{{.Version}}"
{{range .Targets}}
  on_{{if eq .OS "darwin"}}macos{{else}}linux{{end}} do
    on_{{if eq .Arch "arm64"}}arm{{else}}intel{{end}} do
      url "https://github.com/s4na/fragment/releases/download/v{{$.Version}}/fragment_{{$.Version}}_{{.OS}}_{{.Arch}}.tar.gz"
      sha256 "{{.SHA}}"
    end
  end
{{end}}
  def install
    bin.install "fragment"
  end

  test do
    assert_equal version.to_s, JSON.parse(shell_output("#{bin}/fragment version"))["version"]
  end
end
`

func manifest(tag, output string) error {
	var checksums strings.Builder
	entries := append([]target(nil), targets...)
	for i, entry := range entries {
		name := archiveName(tag, entry)
		file, err := os.Open(filepath.Join(output, name))
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		file.Close()
		if err != nil {
			return err
		}
		entries[i].SHA = fmt.Sprintf("%x", hash.Sum(nil))
		fmt.Fprintf(&checksums, "%s  %s\n", entries[i].SHA, name)
	}
	if err := os.WriteFile(filepath.Join(output, "checksums.txt"), []byte(checksums.String()), 0o644); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(output, "fragment.rb"))
	if err != nil {
		return err
	}
	defer file.Close()
	tmpl := template.Must(template.New("formula").Parse(formulaTemplate))
	if err = tmpl.Execute(file, struct {
		Version string
		Targets []target
	}{strings.TrimPrefix(tag, "v"), entries}); err != nil {
		return err
	}
	return file.Close()
}

func run(args []string) error {
	if len(args) == 1 && (args[0] == "next" || args[0] == "latest") {
		latest, err := latestVersion(os.Stdin)
		if err != nil {
			return err
		}
		if args[0] == "next" {
			if latest[2] == ^uint64(0) {
				return fmt.Errorf("patch version overflow")
			}
			latest[2]++
		}
		fmt.Println(latest.String())
		return nil
	}
	if len(args) == 0 || (args[0] != "build" && args[0] != "formula") {
		return fmt.Errorf("usage: release next|latest|build|formula")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	tag := flags.String("version", "", "vMAJOR.MINOR.PATCH")
	commit := flags.String("commit", "", "40-character source commit")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if _, err := parseVersion(*tag); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	output := filepath.Join("bin", "release", *tag)
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	if args[0] == "formula" {
		return manifest(*tag, output)
	}
	if !commitPattern.MatchString(*commit) {
		return fmt.Errorf("invalid source commit")
	}
	actual, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(actual)) != *commit {
		return fmt.Errorf("source checkout does not match --commit")
	}
	status, err := exec.Command("git", "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil || len(status) != 0 {
		return fmt.Errorf("release build requires a clean source checkout; commit changes first")
	}
	return build(*tag, *commit, output)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
