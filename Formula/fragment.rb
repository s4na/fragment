# Generated after a PR merge. Do not edit release URLs or checksums manually.
class Fragment < Formula
  desc "Local Markdown documents shared by CLI, MCP, and Web UI"
  homepage "https://github.com/s4na/fragment"
  version "0.0.1"

  on_macos do
    on_arm do
      url "https://github.com/s4na/fragment/releases/download/v0.0.1/fragment_0.0.1_darwin_arm64.tar.gz"
      sha256 "aa0f2e97cdf3134f2e67ef5b4c3a1d759633edbbf26507312109cc975ab91145"
    end
  end

  on_macos do
    on_intel do
      url "https://github.com/s4na/fragment/releases/download/v0.0.1/fragment_0.0.1_darwin_amd64.tar.gz"
      sha256 "13d87026652404db8aa103c1965b76f77f70563bb052500d42c0e71396946d68"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/s4na/fragment/releases/download/v0.0.1/fragment_0.0.1_linux_arm64.tar.gz"
      sha256 "01d276b02ea73628d8b39266c3ea94f26229131773ea3828c4b223e864553dcb"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/s4na/fragment/releases/download/v0.0.1/fragment_0.0.1_linux_amd64.tar.gz"
      sha256 "7ebdbdf26e8c120b3daa8ccc1c1ac4d44735695cbb29d5d6c35cc54156fd5196"
    end
  end

  def install
    bin.install "fragment"
  end

  test do
    assert_equal version.to_s, JSON.parse(shell_output("#{bin}/fragment version"))["version"]
  end
end
