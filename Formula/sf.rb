class Sf < Formula
  desc "Terminal-first macOS maintenance, app uninstaller, and disk analysis toolkit"
  homepage "https://github.com/SadeepaNHerath/Sift"
  version "2.2.0"
  license "MIT"
  head "https://github.com/SadeepaNHerath/Sift.git", branch: "main"

  depends_on "go" => :build if build.head?
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/SadeepaNHerath/Sift/releases/download/v#{version}/Spacefold_#{version}_darwin_arm64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000001"
    end
    on_intel do
      url "https://github.com/SadeepaNHerath/Sift/releases/download/v#{version}/Spacefold_#{version}_darwin_amd64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000002"
    end
  end

  def install
    if build.head?
      system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=v#{version}"), "./cmd/sf"
    else
      bin.install "sf"
    end

    generate_completions_from_executable(bin/"sf", "completion")
  end

  def caveats
    <<~EOS
      Sift (sf) requires non-root user execution. Never run with sudo sf.
      Elevate privileges with Touch ID via:
        sf touchid enable
    EOS
  end

  test do
    assert_match "Sift (sf)", shell_output("#{bin}/sf --help")
  end
end
