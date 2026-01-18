# Kamui CLI

Command-line interface for [Kamui Platform](https://kamui-platform.com) - a PaaS for deploying and managing applications.

## Installation

### Using Go

```bash
go install github.com/kamui-project/kamui-cli/cmd/kamui@latest
```

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/kamui-project/kamui-cli/releases).

#### macOS

```bash
# Intel
curl -L https://github.com/kamui-project/kamui-cli/releases/latest/download/kamui_darwin_amd64.tar.gz | tar xz
sudo mv kamui /usr/local/bin/

# Apple Silicon
curl -L https://github.com/kamui-project/kamui-cli/releases/latest/download/kamui_darwin_arm64.tar.gz | tar xz
sudo mv kamui /usr/local/bin/
```

#### Linux

```bash
# x86_64
curl -L https://github.com/kamui-project/kamui-cli/releases/latest/download/kamui_linux_amd64.tar.gz | tar xz
sudo mv kamui /usr/local/bin/

# ARM64
curl -L https://github.com/kamui-project/kamui-cli/releases/latest/download/kamui_linux_arm64.tar.gz | tar xz
sudo mv kamui /usr/local/bin/
```

#### Windows

Download `kamui_windows_amd64.zip` from [GitHub Releases](https://github.com/kamui-project/kamui-cli/releases) and add to your PATH.

### Using Homebrew (macOS/Linux)

```bash
brew tap kamui-project/tap
brew install kamui
```

## Quick Start

1. **Login** to your Kamui account:

```bash
kamui login
```

This will open a browser window for authentication.

2. **List** your projects:

```bash
kamui projects list
```

3. **Logout** when done:

```bash
kamui logout
```

## Commands

### Authentication

| Command | Description |
|---------|-------------|
| `kamui login` | Authenticate with Kamui Platform |
| `kamui logout` | Clear stored credentials |

### Projects

| Command | Description |
|---------|-------------|
| `kamui projects list` | List all projects |

### Global Flags

| Flag | Description |
|------|-------------|
| `-o, --output` | Output format: `text` (default) or `json` |
| `-h, --help` | Show help for any command |
| `-v, --version` | Show version information |

## Configuration

Credentials are stored in `~/.kamui/config.json`. This file contains your OAuth tokens and should be kept secure.

## Development

### Prerequisites

- Go 1.21 or later

### Building from Source

```bash
git clone https://github.com/kamui-project/kamui-cli.git
cd kamui-cli
go build -o kamui ./cmd/kamui
```

### Running Tests

```bash
go test ./...
```

### Project Structure

```
kamui-cli/
├── cmd/kamui/          # Entry point
├── internal/
│   ├── api/            # HTTP client for Kamui API
│   ├── auth/           # OAuth authentication
│   ├── cmd/            # CLI commands (interface layer)
│   ├── config/         # Configuration management
│   └── service/        # Business logic (service layer)
├── .goreleaser.yaml    # Release configuration
└── go.mod
```

## License

MIT License - see [LICENSE](LICENSE) for details.
