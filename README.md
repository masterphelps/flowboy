# Flowboy 3000

A Pip-Boy inspired NetFlow traffic generator.

## What It Does

Generates synthetic NetFlow v9 and IPFIX traffic for testing collectors. Configure fictitious machines, define flows between them, and watch pixel-art packets fly across a network map.

The Go module is `github.com/masterphelps/flowboy`.

## Features

- NetFlow v9 + IPFIX export
- Multiple simultaneous collector targets
- App ID support (NBAR)
- Configurable flow rates (Mbps, Gbps, GB/day)
- Realistic export intervals (active/inactive timeouts)
- TUI with Bubbletea (Pip-Boy green phosphor aesthetic)
- Web UI with CRT bezel, scan lines, pixel-art network map
- Space Invaders style animated packet sprites
- YAML config (hand-editable)
- Single binary deployment (embedded web assets)

## Quick Start

```bash
go build -o flowboy ./cmd/flowboy

./flowboy                          # TUI mode (default)
./flowboy --web                    # Web UI at http://localhost:8042
./flowboy --web --port 9090        # Custom port
./flowboy --both                   # TUI + Web simultaneously
./flowboy --headless               # No UI, just generate traffic
./flowboy --config path/to/config  # Custom config file
```

## Config File

Default config lives at `configs/flowboy.yaml`:

```yaml
machines:
  - name: web-server-01
    ip: 192.168.50.201
    mask: 24
  - name: db-server-01
    ip: 10.70.22.45
    mask: 24

flows:
  - name: web-to-db
    source: web-server-01
    source_port: 46578
    destination: db-server-01
    destination_port: 5432
    protocol: TCP
    rate: 90Mbps
    enabled: true

collectors:
  - name: lab-collector
    address: 10.0.0.5:2055
    version: v9
```

Rates accept: `Kbps`, `Mbps`, `Gbps`, `KB/day`, `MB/day`, `GB/day`, `TB/day`.

Collector versions: `v9` (NetFlow v9) or `ipfix`.

## Architecture

```
cmd/flowboy/          CLI entry point, flag parsing, mode selection
internal/
  config/             YAML loading, data model types, rate parsing
  engine/             Flow runner goroutines, NetFlow v9/IPFIX encoding, UDP exporter
  tui/                Bubbletea app: machine list, flow panel, collector status bar
  web/                HTTP server, REST API, WebSocket broadcast, embedded static files
    static/           HTML/CSS/JS for the CRT-themed web UI
configs/              Default flowboy.yaml
tests/                Integration tests
```

The engine spawns one goroutine per flow, generating NetFlow data records at each flow's active timeout interval. Records are fanned out to all configured collectors via UDP. Both the TUI and Web UI read real-time stats from the engine over channels (TUI) or WebSocket (Web).
