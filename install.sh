#!/bin/bash

set -e

# GitHub repository information
REPO="capswan/arc-k1space"
BINARY_NAME="k1space"

# Determine system information
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ARCH="arm64"
fi

# Construct the download URL
LATEST_RELEASE=$(curl -sL https://api.github.com/repos/$REPO/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST_RELEASE/${BINARY_NAME}_${OS}_${ARCH}"

echo "Downloading $BINARY_NAME..."
curl -sL "$DOWNLOAD_URL" -o "$BINARY_NAME"

echo "Installing $BINARY_NAME..."
chmod +x "$BINARY_NAME"
sudo mv "$BINARY_NAME" /usr/local/bin/

echo "$BINARY_NAME has been installed successfully!"