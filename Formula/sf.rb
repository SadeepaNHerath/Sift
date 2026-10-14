class Sf < Formula
  desc "Terminal-first macOS maintenance, app uninstaller, and disk analysis toolkit"
  homepage "https://github.com/SadeepaNHerath/Sift"
  license "MIT"
  head "https://github.com/SadeepaNHerath/Sift.git", branch: "main"

  depends_on "go" => :build if build.head?
  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/SadeepaNHerath/Sift/releases/download/v2.2.0/Spacefold_2.2.0_darwin_arm64.tar.gz"
      sha256 "c818a516e934664ce3f93699c3c6b262b1700512249706d05aa6eec6928e6965"
    end
    on_intel do
      url "https://github.com/SadeepaNHerath/Sift/releases/download/v2.2.0/Spacefold_2.2.0_darwin_amd64.tar.gz"
      sha256 "dab5da07df66dd815c4dae059ce8a73fdfe515614595877cdacd59fad05063ce"
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
