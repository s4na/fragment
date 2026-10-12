# Generated after a PR merge. Do not edit release URLs or checksums manually.
class Fragment < Formula
  desc "Local Markdown documents shared by CLI, MCP, and Web UI"
  homepage "https://github.com/s4na/fragment"
  version "0.0.4"

  on_macos do
    on_arm do
      url "https://github.com/s4na/fragment/releases/download/v0.0.4/fragment_0.0.4_darwin_arm64.tar.gz"
      sha256 "75151e46a42609881d42a083d2add058a0e407c4e4bc368a67ce87f1dd0f01c2"
    end
  end

  on_macos do
    on_intel do
      url "https://github.com/s4na/fragment/releases/download/v0.0.4/fragment_0.0.4_darwin_amd64.tar.gz"
      sha256 "1c4f2c6682d759877c4fc9ac754f6f2c6dc6072d45ca0fc6a603a5d188496dba"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/s4na/fragment/releases/download/v0.0.4/fragment_0.0.4_linux_arm64.tar.gz"
      sha256 "53ac150a56b2e1616c8b96bbebcf61cba653db6cfe86d95f47650ffd1d38d36b"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/s4na/fragment/releases/download/v0.0.4/fragment_0.0.4_linux_amd64.tar.gz"
      sha256 "d6b9ed1c365626ab8ade17458a27aeb77915ee1a2d940250eb731d016af2e356"
    end
  end

  def install
    bin.install "fragment"
  end

  test do
    assert_equal version.to_s, JSON.parse(shell_output("#{bin}/fragment version"))["version"]
  end
end
