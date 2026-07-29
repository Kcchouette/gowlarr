# Gowlarr

**Legal Disclaimer / Non-Affiliation**

Gowlarr is a personal tool, independently developed, that is **not affiliated,
approved, or sponsored** by the [Prowlarr](https://github.com/Prowlarr/Prowlarr),
[Servarr](https://wiki.servarr.com/), or any other \*arr ecosystem project.

Gowlarr is a neutral automation tool: it provides a technical means to search
and resolve links (torrent/magnet/nzb) on third-party sites that **you**
configure. Gowlarr does not store, host, or distribute any copyright-protected
content. **You are solely responsible for how you use this tool and the legality,
in your jurisdiction, of using it with the indexer sites you choose to connect.**

Indexer definitions (Cardigann format, YAML) are **never redistributed** with
this project: they are fetched at runtime, on demand by the user, from the
public [Prowlarr/Indexers](https://github.com/Prowlarr/Indexers) repository,
which remains the property of its respective authors.

## Installation

```bash
# From source (Go 1.25+ required)
go install github.com/Kcchouette/gowlarr/cmd/gowlarr@latest

# Or from a cloned repo
git clone https://github.com/Kcchouette/gowlarr.git
cd gowlarr
go build ./cmd/gowlarr
```

## Usage

### Initialize configuration

```bash
gowlarr config init
```

Creates the default configuration file (`~/.config/gowlarr/config.json` or
`%APPDATA%/gowlarr/config.json` on Windows) and the SQLite database.

### Synchronize indexer definitions

```bash
gowlarr defs sync          # Download Cardigann definitions from GitHub
gowlarr defs list           # List available definitions
gowlarr defs show <id>      # Show definition details
```

### Manage indexers

```bash
gowlarr indexer add <definition-id>           # Add an indexer
gowlarr indexer list                          # List configured indexers
gowlarr indexer test <id>                     # Test connectivity
gowlarr indexer enable/disable <id>          # Enable/disable
gowlarr indexer remove <id>                   # Remove
```

### Search

```bash
gowlarr search "ubuntu"                                  # Simple search
gowlarr search "ubuntu" --indexer 1337x                  # Specific indexer
gowlarr search "ubuntu" --protocol torrent               # Filter by protocol
gowlarr search "ubuntu" --categories 2000,2010           # Filter by category
gowlarr search "ubuntu" --json                           # JSON output
```

### Download

```bash
gowlarr download <result-id>                # Download a result
gowlarr download <result-id> -o ./out.torrent  # Save to file
gowlarr download <result-id> --stdout       # Print to stdout
```

### Usenet (Newznab)

```bash
gowlarr search "query" --newznab-url https://indexer.example.com --newznab-apikey YOUR_KEY
```

## Available Commands

| Command | Description |
|---------|-------------|
| `config init` | Create default configuration |
| `config show` | Show active configuration |
| `defs sync` | Synchronize definitions from GitHub |
| `defs list` | List available definitions |
| `defs show <id>` | Show definition details |
| `indexer add <id>` | Add an indexer |
| `indexer list` | List configured indexers |
| `indexer test <id>` | Test indexer connectivity |
| `indexer enable/disable <id>` | Enable/disable an indexer |
| `indexer remove <id>` | Remove an indexer |
| `search <query>` | Search indexers |
| `download <id>` | Download a result |

## Architecture

```
cmd/gowlarr/          CLI entry point
internal/cli/         Cobra commands
internal/config/      Configuration management
internal/store/       SQLite persistence (migrations, indexers, results)
internal/model/       Shared types (ReleaseInfo, Protocol)
internal/search/      Search engine (parallel providers)
internal/download/    Link resolution (magnet, torrent, nzb)
internal/newznab/     Native Newznab client
internal/cardigannadapter/  Adapter for cardigann-go
```

## Dependencies

- [cardigann-go](https://github.com/Kcchouette/cardigann-go) — Cardigann engine (LGPL-3.0)
- [cobra](https://github.com/spf13/cobra) — CLI framework
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — Pure Go SQLite (no cgo)

## Security

> [!IMPORTANT]
> Indexer parameters persisted in the SQLite database (`settings_json`,
> including any credentials/passwords) are currently stored **in plaintext**.
> The encryption infrastructure exists in `internal/crypt` but is not yet
> wired into the normal execution flow (the encryption key remains `nil`).

## License

MIT — see [LICENSE](LICENSE). The license applies only to Gowlarr's own code,
not to third-party Cardigann definitions fetched at runtime.
