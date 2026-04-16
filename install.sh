#!/bin/sh
set -e

# Claude Monitor Installer
# Usage: curl -sSL https://raw.githubusercontent.com/szaher/claude-monitor/main/install.sh | sh

REPO="szaher/claude-monitor"
BINARY_NAME="claude-monitor"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux) OS="linux" ;;
    darwin) OS="darwin" ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "Claude Monitor Installer"
echo "========================"
echo "OS: $OS"
echo "Arch: $ARCH"
echo ""

# Check if we can write to install dir
if [ ! -w "$INSTALL_DIR" ]; then
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
    echo "Installing to $INSTALL_DIR (no sudo)"
    # Check if it's in PATH
    case ":$PATH:" in
        *":$INSTALL_DIR:"*) ;;
        *) echo "WARNING: $INSTALL_DIR is not in your PATH. Add it with:"
           echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
           echo "" ;;
    esac
fi

# Get latest release URL
DOWNLOAD_URL="https://github.com/$REPO/releases/latest/download/${BINARY_NAME}-${OS}-${ARCH}"

echo "Downloading $BINARY_NAME..."
if command -v curl >/dev/null 2>&1; then
    curl -sSL "$DOWNLOAD_URL" -o "$INSTALL_DIR/$BINARY_NAME"
elif command -v wget >/dev/null 2>&1; then
    wget -q "$DOWNLOAD_URL" -O "$INSTALL_DIR/$BINARY_NAME"
else
    echo "Error: curl or wget required"
    exit 1
fi

chmod +x "$INSTALL_DIR/$BINARY_NAME"

echo "Installed to $INSTALL_DIR/$BINARY_NAME"
echo ""

# Run install command
echo "Setting up claude-monitor..."
"$INSTALL_DIR/$BINARY_NAME" install

echo ""
echo "Installation complete! Run 'claude-monitor serve' to start."
