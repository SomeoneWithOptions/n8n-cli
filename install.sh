#!/bin/sh
# Install the latest n8n CLI release into ~/.local/bin.
#
#   curl -fsSL https://raw.githubusercontent.com/SomeoneWithOptions/n8n-cli/main/install.sh | sh
#
# Overrides:
#   N8N_CLI_VERSION      install this tag instead of the latest release
#   N8N_CLI_INSTALL_DIR  install here instead of ~/.local/bin
#
# This script only writes the binary. Credentials and contexts are created by
# `n8n auth login`; nothing under the config directory is touched here.
set -e

REPO="SomeoneWithOptions/n8n-cli"
INSTALL_DIR="${N8N_CLI_INSTALL_DIR:-$HOME/.local/bin}"
BINARY_NAME="n8n"

OS=$(uname -s)
case "$OS" in
  Darwin) OS="darwin" ;;
  Linux)  OS="linux" ;;
  *)
    echo "Error: unsupported OS: $OS"
    echo "On Windows, install with PowerShell instead:"
    echo "  irm https://raw.githubusercontent.com/${REPO}/main/install.ps1 | iex"
    exit 1
    ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)             echo "Error: unsupported architecture: $ARCH"; exit 1 ;;
esac

LATEST="${N8N_CLI_VERSION:-}"
if [ -z "$LATEST" ]; then
  LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
fi
if [ -z "$LATEST" ]; then
  echo "Error: could not determine latest release"
  echo "Pin one with N8N_CLI_VERSION=vX.Y.Z"
  exit 1
fi

URL="https://github.com/${REPO}/releases/download/${LATEST}/n8n-${OS}-${ARCH}"

echo "Downloading n8n ${LATEST} (${OS}/${ARCH})..."
mkdir -p "$INSTALL_DIR"
curl -fsSL "$URL" -o "${INSTALL_DIR}/${BINARY_NAME}"
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

echo "Installed to ${INSTALL_DIR}/${BINARY_NAME}"
echo "Next: n8n auth login --url https://your-n8n-instance"

case ":$PATH:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    echo "Warning: ${INSTALL_DIR} is not in your PATH."
    SHELL_NAME=$(basename "${SHELL:-sh}")
    case "$SHELL_NAME" in
      fish)
        echo "Add it with:"
        echo "  fish_add_path $INSTALL_DIR"
        ;;
      zsh)
        echo "Add it to your ~/.zshrc:"
        echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.zshrc"
        ;;
      bash)
        echo "Add it to your ~/.bashrc or ~/.bash_profile:"
        echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.bashrc"
        ;;
      *)
        echo "Add it to your shell configuration:"
        echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
        ;;
    esac
    ;;
esac
