# k1space

k1space is a command-line tool designed to streamline the process of creating and managing cloud configurations for Kubernetes clusters created by [kubefirst](https://kubefirst.io). It provides an interactive interface to configure various cloud providers, generate necessary scripts for cluster provisioning, and manage Kubefirst repositories.

## Features

- Support for multiple cloud providers (currently Civo and DigitalOcean)
- Interactive configuration menu for easy setup
- Automatic retrieval of cloud regions and node types
- Generation of configuration files and initialization scripts
- Integration with Kubefirst for Kubernetes cluster provisioning
- Management of Kubefirst repositories ([kubefirst](https://github.com/konstructio/kubefirst), [console](https://github.com/konstructio/console), [kubefirst-api](https://github.com/konstructio/kubefirst-api))
- Cluster provisioning capabilities
- Version management and upgrade functionality

## Installation

### Option 1: Using the install script

You can install k1space using our install script, which will download the latest release and set it up on your system:

```bash
curl -fsSL https://raw.githubusercontent.com/ssotops/k1space/master/install.sh | bash
```

### Option 2: Manual installation

1. Go to the [Releases](https://github.com/ssotops/k1space/releases) page of the k1space repository.
2. Download the appropriate binary for your operating system and architecture.
3. Rename the binary to `k1space` (or `k1space.exe` on Windows).
4. Make the binary executable (`chmod +x k1space` on Unix-based systems).
5. Move the binary to a directory in your PATH.

## Usage

To start using k1space, run the following command in your terminal:

```bash
k1space
```

This will launch the interactive menu where you can:

1. Manage cloud configurations
2. Set up and manage Kubefirst repositories
3. Provision Kubernetes clusters
4. Perform k1space-specific operations

Follow the on-screen prompts to navigate through the various options and configure your environment.

## Configuration

k1space stores its configuration files in the following directory:

```
~/.ssot/k1space/
```

This directory contains:

- `config.hcl`: Stores information about available configurations
- `clouds.hcl`: Contains data about cloud providers, regions, and node types
- Cloud-specific subdirectories with generated scripts and environment files

## Required Environment Variables

Before using k1space to provision clusters, ensure the following environment variables are set:

1. `GITHUB_TOKEN`: A GitHub personal access token with the necessary scopes. You can [Create a GitHub Token with Selected Scopes](https://github.com/settings/tokens/new?scopes=repo,workflow,write:packages,admin:org,admin:public_key,admin:repo_hook,admin:org_hook,user,delete_repo,admin:ssh_signing_key) here. If you want to read more about the scopes that kubefirst uses, you can read more on their public docs [here](https://docs.kubefirst.io/common/gitAuth). If that link has expired or changed, visit the [Github repository responsible for their public docs](https://github.com/konstructio/kubefirst-docs).

2. Cloud Provider-specific tokens:
   - For Civo: `CIVO_TOKEN`
   - For DigitalOcean: `DIGITALOCEAN_TOKEN`

You can set these environment variables in your shell profile or export them before running k1space:

```bash
export GITHUB_TOKEN=your_github_token_here
export CIVO_TOKEN=your_civo_token_here
export DIGITALOCEAN_TOKEN=your_digitalocean_token_here
```

## Main Features

### Config Management

- Create new cloud configurations
- List existing configurations
- Delete specific configurations
- Delete all configurations

### Kubefirst Repository Management

- Clone Kubefirst repositories (kubefirst, console, kubefirst-api)
- Sync repositories to latest changes
- Set up Kubefirst environment
- Run Kubefirst repositories locally
- Revert repositories to main branch

### Cluster Management

- Provision new Kubernetes clusters using Kubefirst
- View cluster provisioning logs

### k1space Operations

- Upgrade k1space to the latest version
- Print configuration paths
- Display version information

## Uninstallation

To uninstall k1space, you can use the provided uninstall script:

```bash
curl -fsSL https://raw.githubusercontent.com/ssotops/k1space/master/uninstall.sh | bash
```

This script will remove the k1space binary and optionally delete the configuration directory.
