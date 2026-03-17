# Flowboy 3000

Network flow simulator with TUI, Web GUI, and headless modes. Generates NetFlow v9/IPFIX traffic between virtual machines and exports to real collectors.

## Quick Reference

```bash
go build -o flowboy ./cmd/flowboy/       # Build
go test ./...                              # Run all tests
./flowboy                                  # TUI mode (default)
./flowboy -web                             # Web UI on :8042
./flowboy -both                            # TUI + Web UI
./flowboy -headless                        # Background/daemon mode
./flowboy -config configs/Testboy.yaml     # Specify config
./flowboy -port 9090                       # Custom web port
```

## Architecture

```
cmd/flowboy/main.go          Entry point, mode selection
internal/
  config/                     YAML config loading/saving, types (Machine, Flow, Collector, Rate)
  engine/
    engine.go                 Flow runner goroutines, stats channel
    exporter.go               UDP export to collectors (v9/IPFIX)
    templates.go              NetFlow v9 template packets
    ipfix.go                  IPFIX template/data packets
  tui/
    app.go                    Top-level bubbletea model, view modes, all message wiring
    machines.go               Machine panel: list, add/edit/delete, CSV import, selection
    flows.go                  Flow panel: list, add/edit/delete, sort, filter, toggle, start/stop
    collectors.go             Collector panel: list, add/edit/delete, live stats
    netmap.go                 ASCII network topology map view
    configpanel.go            Config file management (open/save/save-as/new)
    style.go                  Pip-Boy color palette and lipgloss styles
  web/
    server.go                 HTTP server, WebSocket broadcast, routes
    handlers.go               REST API + WebSocket handlers
    static/                   Web GUI (HTML/CSS/JS)
configs/                      YAML config files
```

## Key Patterns

- **Bubbletea architecture**: Each panel is a sub-model with its own `Update(msg)` and `View()`. Panels emit command messages (e.g. `FlowChangedMsg`, `MachineDeletedMsg`) that `app.go` handles to update the engine and persist config.
- **Engine interaction**: The engine owns machines/flows as maps. CRUD goes through `engine.AddMachine()`, `engine.RemoveFlow()`, etc. Individual flow toggle uses remove+re-add pattern (same as web handlers).
- **Config persistence**: `saveConfig()` in app.go rebuilds the full config from panel state on every mutation. Best-effort write, errors silently ignored in TUI.
- **Stats channels**: Engine emits `FlowStats` on a channel. Exporter stats are polled every 1s. Both feed into TUI panels.
- **Web GUI parity**: TUI and Web GUI should have identical functionality. Both use the same engine/config/exporter layer.

## TUI Keybindings

| Key | Context | Action |
|-----|---------|--------|
| Tab/Shift+Tab | Global | Cycle panel focus |
| q, Ctrl+C | Global | Quit |
| m | Global (normal) | Network map view |
| f | Global (normal) | File/config management |
| Esc | Map/Config view | Back to dashboard |
| j/k, Up/Down | Any list | Navigate |
| n | Any panel | New item |
| e | Machines/Flows/Collectors | Edit selected |
| d | Any panel | Delete selected |
| Enter | Machines | Toggle selection (cross-filters flows) |
| i | Machines | CSV bulk import |
| Space | Flows | Toggle individual flow on/off |
| s | Flows | Start all flows |
| x | Flows | Stop all flows |
| o | Flows | Toggle sort (SRC/DST) |

## Config Format

YAML files in `configs/`. Structure:
```yaml
machines:
  - name: web-server
    ip: 10.0.1.10
    mask: 24
flows:
  - name: web-to-db
    source: web-server
    source_port: 443
    destination: db-server
    destination_port: 5432
    protocol: TCP
    rate: 1Mbps
    enabled: true
collectors:
  - name: collector-01
    address: 10.0.1.50:2055
    version: v9
```

Rate formats: `Kbps`, `Mbps`, `Gbps`, `KB/day`, `MB/day`, `GB/day`, `TB/day`.

## Dependencies

- Go 1.26.1+
- charmbracelet/bubbletea + bubbles + lipgloss (TUI framework)
- gopkg.in/yaml.v3 (config)
- No external runtime dependencies — single static binary
