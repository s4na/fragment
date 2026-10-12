# Generated after a PR merge. Do not edit release URLs or checksums manually.
class Fragment < Formula
  desc "Local Markdown documents shared by CLI, MCP, and Web UI"
  homepage "https://github.com/s4na/fragment"
  version "0.0.3"

  on_macos do
    on_arm do
      url "https://github.com/s4na/fragment/releases/download/v0.0.3/fragment_0.0.3_darwin_arm64.tar.gz"
      sha256 "821afc9c9058090a3a76d65b56dbfba7050f228d9f0e88206fa40387cb19e5af"
    end
  end

  on_macos do
    on_intel do
      url "https://github.com/s4na/fragment/releases/download/v0.0.3/fragment_0.0.3_darwin_amd64.tar.gz"
      sha256 "e11b6a92f826279ababb69a75cb40e700338be852ad853ea2e7290257e8fa0d2"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/s4na/fragment/releases/download/v0.0.3/fragment_0.0.3_linux_arm64.tar.gz"
      sha256 "ac33df2b1ea74c7af9452322ad5e82bb46690bb0fb0fa13511330570ea48802b"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/s4na/fragment/releases/download/v0.0.3/fragment_0.0.3_linux_amd64.tar.gz"
      sha256 "748e16ffc1d8f2be46aa744843ecf5a3ada2a56b6e1faff582862778e76eb3a3"
    end
  end

  def install
    bin.install "fragment"
  end

  test do
    assert_equal version.to_s, JSON.parse(shell_output("#{bin}/fragment version"))["version"]
  end
end
