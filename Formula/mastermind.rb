class Mastermind < Formula
  desc "Orchestrate multiple Claude Code agents in parallel with tmux and git worktrees"
  homepage "https://github.com/simonbystrom/mastermind"
  url "https://github.com/simonbystrom/mastermind.git",
      tag:      "v0.1.0",
      revision: ""
  license "MIT"
  head "https://github.com/simonbystrom/mastermind.git", branch: "main"

  depends_on "go" => :build
  depends_on "tmux"
  depends_on "lazygit"

  def install
    ldflags = "-X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags:)
  end

  def post_install
    system bin/"mastermind", "--init-config"
  end

  test do
    assert_match "mastermind", shell_output("#{bin}/mastermind --version")
  end
end
