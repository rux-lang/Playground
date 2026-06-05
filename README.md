# Rux Playground Setup

Deployment files for the [Rux Language Playground](https://rux-lang.dev/playground) — a sandboxed in-browser playground for the Rux compiler, running on Debian 12 with Docker and Cloudflare Tunnel.

[Alternate Playground URL](https://ruxplayground.dpdns.org)
## Files

| File | Purpose |
|------|---------|
| `setup.sh` | Full deployment script (8 phases) |
| `main.go` | Go HTTP backend serving `/run` and embedded UI |
| `index.html` | Web UI with CodeMirror editor |
| `Dockerfile` | Fedora container with Rux + cached Std/Linux packages |
| `entrypoint.sh` | Container entrypoint — creates project, installs deps, builds, runs |
| `rux-playground.service` | systemd unit for the Go backend |

## Architecture

- **Go backend** serves the web UI and handles POST `/run` by spawning a Docker container
- **Fedora container** runs the Rux compiler with `--cap-drop=ALL`, `--network=none`, 30s timeout
- **Cloudflare Tunnel** provides HTTPS without opening firewall ports

## Quick Start

```bash
# Full deploy
sudo bash setup.sh all

# Upgrade everything
sudo bash setup.sh 9
```

See `setup.sh` for individual phase numbers.

## License

[MIT](LICENSE)

