# Generated after a PR merge. Do not edit release URLs or checksums manually.
class Fragment < Formula
  desc "Local Markdown documents shared by CLI, MCP, and Web UI"
  homepage "https://github.com/s4na/fragment"
  version "0.0.5"

  on_macos do
    on_arm do
      url "https://github.com/s4na/fragment/releases/download/v0.0.5/fragment_0.0.5_darwin_arm64.tar.gz"
      sha256 "908ab865a8bafc6d0c3c9f8c73029d980bb77db308375b221411a02b535165cb"
    end
  end

  on_macos do
    on_intel do
      url "https://github.com/s4na/fragment/releases/download/v0.0.5/fragment_0.0.5_darwin_amd64.tar.gz"
      sha256 "37db566bacc45622e55921266d408e835581e882e19269933917281e1e5a16a4"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/s4na/fragment/releases/download/v0.0.5/fragment_0.0.5_linux_arm64.tar.gz"
      sha256 "1578d70e41031597a2217265d5be2a55825688c57dfb08246fe3093a35e2738b"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/s4na/fragment/releases/download/v0.0.5/fragment_0.0.5_linux_amd64.tar.gz"
      sha256 "c15ac95d7227d145b847a54ef360a5d64041f3bb72a33ae722ee72e8913f8a47"
    end
  end

  def install
    bin.install "fragment"
  end

  test do
    assert_equal version.to_s, JSON.parse(shell_output("#{bin}/fragment version"))["version"]
  end
end
