# k1space

k1space is a command-line tool designed to streamline the process of creating and managing cloud configurations for Kubernetes clusters created by [kubefirst](https://kubefirst.io). It provides an interactive interface to configure various cloud providers and generate the necessary scripts for cluster provisioning.

## Features

- Support for multiple cloud providers (currently Civo and DigitalOcean)
- Interactive configuration menu for easy setup
- Automatic retrieval of cloud regions and node types
- Generation of configuration files and initialization scripts
- Integration with Kubefirst for Kubernetes cluster provisioning

## Installation

### Option 1: Using the install script

You can install k1space using our install script, which will download the latest release and set it up on your system:

```bash
curl -fsSL https://raw.githubusercontent.com/ssotops/k1space/main/install.sh | bash
```

### Option 2: Manual installation

1. Go to the [Releases](https://github.com/ssotops/k1space/releases) page of the k1space repository.
2. Download the appropriate binary for your operating system and architecture.
3. Rename the binary to `k1space` (or `k1space.exe` on Windows).
4. Make the binary executable (`chmod +x k1space` on Unix-based systems).
5. Move the binary to a directory in your PATH.

## Usage

To start using k1space, simply run the following command in your terminal:

```bash
k1space
```

This will launch the interactive menu where you can:

1. Create and manage cloud configurations
2. Set up Kubefirst repositories
3. Provision Kubernetes clusters

Follow the on-screen prompts to configure your cloud provider, select regions, and set up your cluster.

## Configuration

k1space stores its configuration files in the following directory:

```
~/.ssot/k1space/
```

This directory contains:

- `index.hcl`: Stores information about available configurations
- `clouds.hcl`: Contains data about cloud providers, regions, and node types
- Cloud-specific subdirectories with generated scripts and environment files

## Uninstallation

To uninstall k1space, you can use the provided uninstall script:

```bash
curl -fsSL https://raw.githubusercontent.com/ssotops/k1space/main/uninstall.sh | bash
```

This script will remove the k1space binary and optionally delete the configuration directory.

## Support

If you encounter any issues or have questions about k1space, please [open an issue](https://github.com/ssotops/k1space/issues) on our GitHub repository.
