class Sift < Formula
  desc "Terminal-first macOS maintenance, app uninstaller, and disk analysis toolkit"
  homepage "https://github.com/SadeepaNHerath/Sift"
  url "https://github.com/SadeepaNHerath/Sift/archive/refs/tags/v2.2.0.tar.gz"
  sha256 "be78604c2b63da18a412dd67ec9ae018a1aafb28dc4a0d172e53992598402eca"
  license "MIT"
  head "https://github.com/SadeepaNHerath/Sift.git", branch: "main"

  depends_on "go" => :build
  depends_on :macos

  conflicts_with "sf", because: "both install an 'sf' binary"

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=v#{version}"), "./cmd/sf"
    bin.install_symlink bin/"sift" => "sf"

    generate_completions_from_executable(bin/"sift", "completion")
  end

  def caveats
    <<~EOS
      Sift (sf) requires non-root user execution. Never run with sudo sf.
      Elevate privileges with Touch ID via:
        sf touchid enable
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sift --version")
    assert_match "Sift (sf)", shell_output("#{bin}/sf --help")
  end
end
