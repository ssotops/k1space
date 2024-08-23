#!/bin/bash

set -e

BINARY="k1space"
CONFIG_DIR="$HOME/.ssot/k1space"

# Determine OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

# Function to prompt user for yes/no input
prompt_yes_no() {
    while true; do
        read -p "$1 (y/n): " yn
        case $yn in
            [Yy]* ) return 0;;
            [Nn]* ) return 1;;
            * ) echo "Please answer yes or no.";;
        esac
    done
}

# Uninstall binary
if [ "$OS" = "windows" ]; then
    echo "For Windows, please manually delete the k1space.exe file from your PATH."
else
    if [ -f "/usr/local/bin/$BINARY" ]; then
        sudo rm "/usr/local/bin/$BINARY"
        echo "$BINARY has been uninstalled."
    else
        echo "$BINARY was not found in /usr/local/bin."
    fi
fi

# Remove configuration directory if it exists
if [ -d "$CONFIG_DIR" ]; then
    if prompt_yes_no "Do you want to remove the configuration directory ($CONFIG_DIR)?"; then
        rm -rf "$CONFIG_DIR"
        echo "Configuration directory has been removed."
    else
        echo "Configuration directory was not removed."
    fi
fi

# Additional cleanup steps (if applicable)
# For example, removing any created symlinks, cached data, etc.
# Add these steps here if needed for k1space

echo "Uninstallation complete."
