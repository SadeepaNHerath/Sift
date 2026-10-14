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
      sha256 "0e293deb22baf24f141355588584741f92a12ee32067d544fee57876c91359c0"
    end
    on_intel do
      url "https://github.com/SadeepaNHerath/Sift/releases/download/v2.2.0/Spacefold_2.2.0_darwin_amd64.tar.gz"
      sha256 "853ee48a49c36138f4acbed1e7db768d41735c6c583a51a055e67aaca3eb2fdb"
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
