# Generated after a PR merge. Do not edit release URLs or checksums manually.
class Fragment < Formula
  desc "Local Markdown documents shared by CLI, MCP, and Web UI"
  homepage "https://github.com/s4na/fragment"
  version "0.0.2"

  on_macos do
    on_arm do
      url "https://github.com/s4na/fragment/releases/download/v0.0.2/fragment_0.0.2_darwin_arm64.tar.gz"
      sha256 "ec3a873899032af3a4799e6cf4a99f26ed282deaa53a9c7c78bd0fb42ced0bff"
    end
  end

  on_macos do
    on_intel do
      url "https://github.com/s4na/fragment/releases/download/v0.0.2/fragment_0.0.2_darwin_amd64.tar.gz"
      sha256 "19c7fad9d4e74bbdcf6d36a4581c89c8e3dd54b4255bc165d2aa9a7f6f9774e6"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/s4na/fragment/releases/download/v0.0.2/fragment_0.0.2_linux_arm64.tar.gz"
      sha256 "183967dfaf7b5062c949c0bd670e1aa84d533780a8db5f1290de5928d910e602"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/s4na/fragment/releases/download/v0.0.2/fragment_0.0.2_linux_amd64.tar.gz"
      sha256 "870ad3fcb8de960d80406087ec753dd22298920a1006284a61d7924b10ebc653"
    end
  end

  def install
    bin.install "fragment"
  end

  test do
    assert_equal version.to_s, JSON.parse(shell_output("#{bin}/fragment version"))["version"]
  end
end
