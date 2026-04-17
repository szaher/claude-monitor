---
layout: default
title: Installation
nav_order: 2
---

# Installation

## Requirements

- **Go 1.25+** (for building from source)
- **CGO enabled** -- required for SQLite3 with FTS5 support
- A C compiler (gcc or clang)

## Build from Source

```sh
git clone https://github.com/szaher/claude-monitor.git
cd claude-monitor
make build
```

The binary is placed in `bin/claude-monitor`.

To install to `/usr/local/bin`:

```sh
make install
```

## Download Pre-built Binary

Download the latest release for your platform from [GitHub Releases](https://github.com/szaher/claude-monitor/releases):

| Platform | Architecture | Download |
|----------|-------------|----------|
| Linux | amd64 | `claude-monitor-linux-amd64.tar.gz` |
| Linux | arm64 | `claude-monitor-linux-arm64.tar.gz` |
| macOS | Intel | `claude-monitor-darwin-amd64.tar.gz` |
| macOS | Apple Silicon | `claude-monitor-darwin-arm64.tar.gz` |

```sh
# Example: macOS Apple Silicon
tar xzf claude-monitor-darwin-arm64.tar.gz
chmod +x claude-monitor
sudo mv claude-monitor /usr/local/bin/
```

## Setup

After installing the binary, run the setup steps:

```sh
# 1. Install Claude Code hooks and initialize the database
claude-monitor install

# 2. Import existing session logs (optional but recommended)
claude-monitor import

# 3. Start the server and open the dashboard
claude-monitor serve
```

The `install` command does the following:

1. Creates the data directory at `~/.claude-monitor/`
2. Initializes the SQLite database with all required tables
3. Registers a Claude Code hook that sends events to the monitor

## Verify Installation

```sh
# Check that the binary works
claude-monitor version

# Check hooks are installed and database is ready
claude-monitor status
```

## Uninstall

```sh
# Remove hooks only
claude-monitor uninstall

# Remove hooks and delete all data
claude-monitor uninstall --delete-data
```
