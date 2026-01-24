# Kamui CLI

[![Release](https://img.shields.io/github/v/release/kamui-project/cli)](https://github.com/kamui-project/cli/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/kamui-project/cli)](https://goreportcard.com/report/github.com/kamui-project/cli)

Command-line interface for [Kamui Platform](https://kamui-platform.com) - a PaaS for deploying and managing applications, databases, and cron jobs with ease.

## Features

- 🔐 **Secure Authentication** - OAuth 2.0 with GitHub SSO
- 📦 **Project Management** - List and manage your projects
- 🖥️ **Cross-Platform** - macOS, Linux, and Windows support
- 📄 **Multiple Output Formats** - Text tables or JSON for scripting

## Installation

### Homebrew (Recommended for macOS/Linux)

```bash
brew install kamui-project/tap/kamui
```

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/kamui-project/cli/releases).

<details>
<summary>macOS</summary>

```bash
# Apple Silicon (M1/M2/M3)
curl -L https://github.com/kamui-project/cli/releases/latest/download/kamui_darwin_arm64.tar.gz | tar xz
sudo mv kamui /usr/local/bin/

# Intel
curl -L https://github.com/kamui-project/cli/releases/latest/download/kamui_darwin_amd64.tar.gz | tar xz
sudo mv kamui /usr/local/bin/
```

</details>

<details>
<summary>Linux</summary>

```bash
# x86_64
curl -L https://github.com/kamui-project/cli/releases/latest/download/kamui_linux_amd64.tar.gz | tar xz
sudo mv kamui /usr/local/bin/

# ARM64
curl -L https://github.com/kamui-project/cli/releases/latest/download/kamui_linux_arm64.tar.gz | tar xz
sudo mv kamui /usr/local/bin/
```

</details>

<details>
<summary>Windows</summary>

Download `kamui_windows_amd64.zip` from [GitHub Releases](https://github.com/kamui-project/cli/releases) and add to your PATH.

</details>

### Using Go

```bash
go install github.com/kamui-project/cli/cmd/kamui@latest
```

## Quick Start

### 1. Login

```bash
kamui login
```

This opens a browser window for GitHub authentication. After successful login, your credentials are stored securely.

### 2. List Projects

```bash
kamui projects list
```

Output:
```
ID                                    NAME            PLAN  REGION  APPS  DATABASES
5f809f2f-0787-40ca-9a43-a3a59edb5400  my-project      free  tokyo   2     1
21065335-ade9-4e63-bfcc-284760fa3957  another-app     pro   osaka   3     0
```

### 3. Get Project Details

```bash
kamui projects get <project-id>
```

### 4. JSON Output (for scripting)

```bash
kamui projects list -o json
```

## Commands

### Authentication

| Command | Description |
|---------|-------------|
| `kamui login` | Authenticate with Kamui Platform via GitHub |
| `kamui logout` | Clear stored credentials |

### Projects

| Command | Description |
|---------|-------------|
| `kamui projects list` | List all projects |
| `kamui projects get <id>` | Get project details by ID |
| `kamui projects create` | Create a new project |
| `kamui projects delete <id>` | Delete a project |

### Apps

| Command | Description |
|---------|-------------|
| `kamui apps list -p <project>` | List all apps in a project |
| `kamui apps create` | Create a new app (dynamic or static) |
| `kamui apps delete <id>` | Delete an app |

The `apps create` command supports three app types:
- **Dynamic app** - Server-side applications (Node.js, Go, Python)
- **Static app (GitHub)** - Static sites from GitHub repository
- **Static app (ZIP upload)** - Static sites from local ZIP file

### Global Flags

| Flag | Description |
|------|-------------|
| `-o, --output` | Output format: `text` (default) or `json` |
| `-h, --help` | Show help for any command |
| `-v, --version` | Show version information |

## Configuration

Credentials are stored in `~/.kamui/config.json`. This file contains your OAuth tokens and should be kept secure.

```bash
# View config location
ls ~/.kamui/
```

## Development

### Prerequisites

- Go 1.21 or later

### Building from Source

```bash
git clone https://github.com/kamui-project/cli.git
cd cli
go build -o kamui ./cmd/kamui
```

### Running Tests

```bash
go test -v ./...
```

### Project Structure

```
cli/
├── cmd/kamui/              # Entry point
├── internal/
│   ├── api/                # HTTP client for Kamui API
│   ├── auth/               # OAuth 2.0 authentication flow
│   ├── cmd/                # CLI commands (interface layer)
│   ├── config/             # Configuration management
│   ├── di/                 # Dependency injection container
│   └── service/
│       ├── interface/      # Service interfaces
│       ├── auth.go         # Auth service implementation
│       └── project.go      # Project service implementation
├── .goreleaser.yaml        # Release configuration
└── go.mod
```

### Architecture

The CLI follows a clean architecture pattern:

- **Interface Layer** (`internal/cmd/`) - Cobra commands, user interaction
- **Service Layer** (`internal/service/`) - Business logic, API orchestration
- **API Layer** (`internal/api/`) - HTTP client for backend communication

Services are injected via a DI container, making the code testable with mocks.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Links

- [Kamui Platform](https://kamui-platform.com)
- [Documentation](https://docs.kamui-platform.com)
- [GitHub Issues](https://github.com/kamui-project/cli/issues)
