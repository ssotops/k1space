#!/bin/bash

set -e

# GitHub repository information
REPO="capswan/arc-k1space"
BINARY_NAME="k1space"

# Check for GITHUB_TOKEN in environment
if [ -z "$GITHUB_TOKEN" ]; then
    echo "Error: GITHUB_TOKEN environment variable is not set."
    echo "Please set it with: export GITHUB_TOKEN=your_token_here"
    exit 1
fi

# Determine system information
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ARCH="arm64"
fi

# Construct the download URL
LATEST_RELEASE=$(curl -sL -H "Authorization: token $GITHUB_TOKEN" https://api.github.com/repos/$REPO/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
DOWNLOAD_URL="https://api.github.com/repos/$REPO/releases/assets/$(curl -sL -H "Authorization: token $GITHUB_TOKEN" https://api.github.com/repos/$REPO/releases/latest | jq ".assets[] | select(.name == \"${BINARY_NAME}_${OS}_${ARCH}\") | .id")"

echo "Downloading $BINARY_NAME..."
curl -sL -H "Authorization: token $GITHUB_TOKEN" -H "Accept: application/octet-stream" "$DOWNLOAD_URL" -o "$BINARY_NAME"

echo "Installing $BINARY_NAME..."
chmod +x "$BINARY_NAME"
sudo mv "$BINARY_NAME" /usr/local/bin/

echo "$BINARY_NAME has been installed successfully!"
