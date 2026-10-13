class Sf < Formula
  desc "Terminal-first macOS maintenance, app uninstaller, and disk analysis toolkit"
  homepage "https://github.com/SadeepaNHerath/Spacefold"
  version "2.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/SadeepaNHerath/Spacefold/releases/download/v#{version}/sf_#{version}_darwin_arm64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000" # Updated upon release
    else
      url "https://github.com/SadeepaNHerath/Spacefold/releases/download/v#{version}/sf_#{version}_darwin_amd64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000" # Updated upon release
    end
  end

  head "https://github.com/SadeepaNHerath/Spacefold.git", branch: "main"

  depends_on "go" => :build if build.head?

  def install
    if build.head?
      system "go", "build", *std_go_args(ldflags: "-s -w -X github.com/SadeepaNHerath/Spacefold/internal/core.Version=#{version}"), "./cmd/sf"
    else
      bin.install "sf"
    end

    # Generate shell completions
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
