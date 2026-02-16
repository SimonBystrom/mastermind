#!/bin/sh
# Mastermind installer
# Usage: curl -fsSL https://raw.githubusercontent.com/simonbystrom/mastermind/main/install.sh | sh
set -e

REPO="simonbystrom/mastermind"
BINARY="mastermind"

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Linux)  OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)
    echo "Error: unsupported OS: $OS" >&2
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# Get latest release tag
echo "Fetching latest release..."
TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"//;s/".*//')"
if [ -z "$TAG" ]; then
  echo "Error: could not determine latest release" >&2
  exit 1
fi
echo "Latest release: $TAG"

# Download tarball
URL="https://github.com/${REPO}/releases/download/${TAG}/${BINARY}-${TAG}-${OS}-${ARCH}.tar.gz"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading ${BINARY}-${TAG}-${OS}-${ARCH}.tar.gz..."
curl -fsSL "$URL" -o "$TMPDIR/$BINARY.tar.gz"
tar -xzf "$TMPDIR/$BINARY.tar.gz" -C "$TMPDIR"

# Install binary
INSTALL_DIR="/usr/local/bin"
if [ -w "$INSTALL_DIR" ]; then
  install -m 755 "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"
else
  if command -v sudo >/dev/null 2>&1; then
    echo "Installing to $INSTALL_DIR (requires sudo)..."
    sudo install -m 755 "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"
  else
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
    install -m 755 "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"
    echo "Installed to $INSTALL_DIR — make sure it's on your PATH"
  fi
fi

# Write default config
"$INSTALL_DIR/$BINARY" --init-config >/dev/null 2>&1 || true

echo ""
echo "mastermind $TAG installed to $INSTALL_DIR/$BINARY"
echo ""
echo "Prerequisites:"
echo "  - tmux 3.0+   brew install tmux"
echo "  - git          (likely already installed)"
echo "  - claude       https://docs.anthropic.com/en/docs/claude-code"
echo "  - lazygit      brew install lazygit"
echo ""
echo "Run 'mastermind' inside a tmux session in any git repo to get started."
