# gh-actions-dash

A TUI dashboard for GitHub Actions

## Overview

gh-actions-dash is a terminal user interface application for monitoring and managing GitHub Actions workflows and workflow runs. It's available as a GitHub CLI extension.

## Features

- Display GitHub Actions workflows
- Monitor workflow runs
- View workflow run logs
- Intuitive keyboard-driven TUI interface
- GitHub CLI authentication integration

## Requirements

- Go 1.24.5 or higher
- GitHub CLI (`gh`) installed
- GitHub authentication configured

## Installation

### As a GitHub CLI extension

```bash
gh extension install ryo246912/gh-actions-dash
```

### Local development environment (using mise)

```bash
# Clone the repository
git clone https://github.com/ryo246912/gh-actions-dash.git
cd gh-actions-dash

# Install dependencies
go mod download

# Install extension locally
mise run extension-local-install
```

## Usage

```bash
# Use current directory repository
gh actions-dash

# Specify a specific repository
gh actions-dash --owner <owner> --repo <repo>
```

### Options

- `--owner`, `-o`: Repository owner
- `--repo`, `-r`: Repository name

## License

MIT License

## Contributing

Pull requests and issue reports are welcome. If you'd like to contribute to development, please create an issue first.
