# Flowboy Design Document

**Date:** 2026-03-12
**Status:** Approved
**App Name:** Flowboy (will rename directory from flowmaster → flowboy)

## Overview

Flowboy is a Go application that generates synthetic NetFlow traffic (v9 + IPFIX) for testing NetFlow collectors. It provides both a TUI (Bubbletea) and a web UI with a Pip-Boy / Vault-Tec inspired CRT aesthetic. Users configure fictitious machines with IPs and subnet masks, then define flows between them. Flowboy generates standards-compliant NetFlow packets and exports them to one or more collectors simultaneously.

The name is a riff on the Pip-Boy from Fallout — the entire UI aesthetic follows this theme with green phosphor CRT screens, scan lines, and pixel-art network visualizations.

## Key Decisions

| Decision | Choice |
|---|---|
| NetFlow versions | v9 + IPFIX |
| Architecture | Monolithic single Go binary |
| Web UI deployment | Embedded web server via `embed.FS` |
| Network model | Fictitious machines with auto-segments from subnet masks |
| Flow pacing | Realistic export intervals (configurable active/inactive timeouts) |
| Config format | YAML |
| Export targets | Multiple simultaneous collectors |
| App ID | Supported in flow records |
| Default web port | 8042 (overridable with --port) |
| UI aesthetic | Pip-Boy / Vault-Tec CRT — green phosphor, scan lines, pixel art |
| Visual style reference | CodePen "Fallout 4 Pip-Boy in CSS" by Stix — colors, fonts, scan lines, CRT glow ONLY (not layout or hardware frame) |

## Data Model

```
Machine
├── name: string          ("web-server-01")
├── ip: net.IP            (192.168.50.201)
├── mask: net.IPMask      (/24)
├── segment: auto-derived (192.168.50.0/24)
└── flows: []Flow         (attached as source or destination)

Flow
├── source: Machine
├── source_port: uint16
├── destination: Machine
├── destination_port: uint16
├── protocol: TCP|UDP|ICMP|etc
├── rate: bandwidth OR volume  ("90Mbps" or "10GB/day")
├── app_id: optional uint32
├── active_timeout: duration   (default 60s)
├── inactive_timeout: duration (default 15s)
└── enabled: bool

Collector
├── name: string
├── address: ip:port
└── protocol_version: v9|ipfix

Segment (auto-generated)
├── cidr: net.IPNet
└── machines: []Machine
```

YAML config maps directly to these structs. Machines are the anchor point for flow configuration — you define a machine once, then attach multiple flows to it as source or destination.

## Flow Engine

### Per-flow goroutine model
- Each enabled flow spawns a goroutine that maintains: byte counter, packet counter, start time, last-active time
- Goroutine sleeps for the active timeout interval, then builds and queues a NetFlow data record with accumulated counters
- Counters are derived from the configured rate (e.g., 90Mbps = ~675MB per 60s active timeout)
- Template records are sent periodically (every N data exports, configurable) per the v9/IPFIX spec

### Exporter fan-out
- Single exporter goroutine reads from a channel, encodes packets, and sends UDP datagrams to all configured collectors simultaneously
- v9 and IPFIX use separate template sets — each collector is tagged with its protocol version and gets the right encoding

### Flow control signals via channels
```
TUI/Web UI → engine: start, stop, create, update, delete flow
Engine → TUI/Web UI: flow stats (bytes sent, packets sent, active/idle state)
```

All state changes persist back to YAML config so you can kill the process and restart with the same setup.

## TUI Design (Bubbletea + Lip Gloss)

Three-panel Pip-Boy screen layout:

```
╔══════════════════════════════════════════════════════╗
║  F L O W B O Y  3000          [STATS] [MAP] [CONFIG]║
╠══════════════════════════════════════════════════════╣
║ MACHINES          │ ACTIVE FLOWS                     ║
║ ▸ web-server-01   │ web-01:46578 → db-01:5432  TCP  ║
║   db-server-01    │ ████████░░ 72Mbps  ~~∿∿~~       ║
║   app-server-01   │                                  ║
║   lb-frontend-01  │ web-01:8080 → app-01:3000  TCP  ║
║                   │ ██████████ 90Mbps  ~~∿∿∿~~      ║
║                   │                                  ║
║ [N]ew [E]dit [D]el│ web-01:53201 → lb-01:443  TCP   ║
║                   │ ███░░░░░░░ 12Mbps  ~∿~          ║
╠══════════════════════════════════════════════════════╣
║ ▸ 2055/UDP → collector-lab.local     OK  ↑ 1.2K pkt ║
║ ▸ 4739/UDP → elastic-dev.local       OK  ↑ 1.2K pkt ║
╚══════════════════════════════════════════════════════╝
```

### Key elements
- **Left panel:** Machine list + CRUD operations (N/E/D keybinds). Highlighting a machine shows its flows
- **Right panel:** Active flows with progress bars for rate + oscilloscope-style waveform animation that pulses based on throughput
- **Bottom bar:** Collector status — connection state, packet counters
- **Top tabs:** STATS (default), MAP (ASCII network map), CONFIG (edit settings inline)

### Pip-Boy aesthetic in TUI
- Green on black via Lip Gloss
- Segmented font for headers
- Box-drawing borders
- Subtle "scan line" effect using alternating dim/normal rows

### Animations
- Waveform characters cycle on a tick (~∿∿~ → ∿∿~∿ → ∿~∿∿) — oscilloscope feel
- Progress bars pulse slightly when active
- New flow start gets a brief "initializing..." flicker

## Web UI Design

### Style Reference (from CodePen "Fallout 4 Pip-Boy in CSS" by Stix)
Used for visual style ONLY — not layout or hardware elements:
- Color palette: `#8df776` green on `#000`, `#272b2a` dark panels, `#d8c99e` accent
- Font: `Droid Sans` (or similar monospace/military), uppercase, 700 weight
- Scan line animation sweeping the screen
- Screen reflection overlay
- CRT bezel with "FLOWBOY 3000" label

### Layout — Full Dashboard (No Tab Navigation)

Three tiers, all visible simultaneously:

```
┌──────────────────── FLOWBOY 3000 ────────────────────────────┐
│                                                              │
│  ┌─ MACHINES ──────────────────┐  ┌─ COLLECTORS ──────────┐ │
│  │ ▸ web-server-01  .50.201/24 │  │ lab.local:2055        │ │
│  │   db-server-01   .22.45/24  │  │ ● OK  ↑1.2K pkt      │ │
│  │   app-server-01  .50.100/24 │  │                       │ │
│  │  [+ Add Machine]            │  │ elastic:4739          │ │
│  └─────────────────────────────┘  │ ● OK  ↑1.2K pkt      │ │
│                                   │ [+ Add Collector]     │ │
│                                   └───────────────────────┘ │
│  ┌─ ACTIVE FLOWS ───────────────────────────────────────────┐│
│  │ web-01:46578 → db-01:5432   TCP  ██████░░ 72M  ~∿∿~    ││
│  │ web-01:8080  → app-01:3000  TCP  █████████ 90M  ~∿∿∿~  ││
│  │ web-01:53201 → lb-01:443    TCP  ███░░░░░ 12M  ~∿~     ││
│  │ [+ New Flow]                      [Start All] [Stop All]││
│  └──────────────────────────────────────────────────────────┘│
│                                                              │
│  ┌─ NETWORK MAP ────────────────────────────────────────────┐│
│  │                                                          ││
│  │  ┌─ 192.168.50.0/24 ─┐        ┌─ 10.70.22.0/24 ──┐    ││
│  │  │ [▣ web-01] ▪▪▪▪▪▪▪▪▪▪▪▪▪▪▪▪│▪▪▪▪ [▣ db-01]    │    ││
│  │  │            ▫···▫···│········│··▫  [▣ cache-01]  │    ││
│  │  │ [▣ app-01] ────────│────────│──────────┘        │    ││
│  │  └────────────────────┘        └───────────────────┘    ││
│  │        ▪▪▪▪▪▪▪▪▪▪▪▪                                    ││
│  │  ┌─ 10.0.0.0/8 ──┐                                     ││
│  │  │ [▣ lb-01]      │                                     ││
│  │  └────────────────┘                                     ││
│  └──────────────────────────────────────────────────────────┘│
│  ↑4.2Gbps total  ●12 active flows  ↑38.4K pkt/s  ENGINE:ON │
└──────────────────────────────────────────────────────────────┘
```

### Three tiers
1. **Top** — Machines (left) + Collectors (right), side by side
2. **Middle** — Active Flows, full width, live throughput + waveforms
3. **Bottom** — Network Map, full width, biggest panel — the star of the show

### Network Map — Space Invaders Meets Pip-Boy

The network map is a pixel-art canvas rendered on a pixel grid (no anti-aliasing, everything snaps to grid):

- **Nodes** (machines): chunky 8-bit pixel-art terminal icons, green phosphor
- **Segments**: dotted pixel borders grouping machines by auto-derived subnet
- **Packets**: tiny pixel sprites traveling along flow lines — Space Invaders bullets style
  - Packet sprite density = throughput (90Mbps = swarm, 1Mbps = occasional blip)
  - TCP = solid square ▪, UDP = hollow square ▫, ICMP = diamond ◆
- **Flow lines**: pixelated stepped paths (not smooth curves)
- **Interactions**: hover/click a flow → packets glow brighter; click a node → highlights in lists above
- Idle flows: dim dashed pixel line, no packets
- Flow start: brief "power up" pixel burst at source node
- Scan lines still sweep over the map panel

### Bidirectional Interactions
- Click machine in list → highlights on map + filters flows
- Click node on map → highlights in machine list + filters flows
- Click flow line on map → highlights in flow panel
- Click flow in panel → highlights source/dest on map

### Tech Stack
- Go `embed.FS` serves a vanilla JS + Canvas SPA
- Tailwind CSS for layout + custom Pip-Boy CRT theme
- WebSocket for real-time flow stats
- Canvas with pixel grid rendering for the network map
- No frontend framework — keeps binary small

## Project Structure

```
flowboy/
├── cmd/
│   └── flowboy/
│       └── main.go              # entry point, CLI flags
├── internal/
│   ├── config/
│   │   ├── config.go            # YAML parsing, validation
│   │   └── types.go             # Machine, Flow, Collector structs
│   ├── engine/
│   │   ├── engine.go            # flow lifecycle, goroutine management
│   │   ├── exporter.go          # v9/IPFIX encoding, UDP fan-out
│   │   └── templates.go         # NetFlow template definitions
│   ├── tui/
│   │   ├── app.go               # bubbletea main model
│   │   ├── machines.go          # machine list panel
│   │   ├── flows.go             # active flows panel + waveforms
│   │   ├── style.go             # lipgloss Pip-Boy theme
│   │   └── collectors.go        # collector status panel
│   └── web/
│       ├── server.go            # HTTP + WebSocket server
│       ├── handlers.go          # REST API for CRUD + engine control
│       └── static/              # embedded SPA
│           ├── index.html
│           ├── css/
│           │   ├── pipboy.css   # CRT effects, bezel, scan lines
│           │   └── app.css      # tailwind + layout
│           └── js/
│               ├── app.js       # main SPA logic
│               ├── map.js       # Canvas pixel-art network map
│               └── ws.js        # WebSocket client for live updates
├── configs/
│   └── flowboy.yaml             # default config file
├── go.mod
└── go.sum
```

## CLI Interface

```
flowboy                        # TUI mode (default)
flowboy --web                  # Web UI on port 8042
flowboy --web --port 9090      # Web UI on custom port
flowboy --both                 # TUI + Web simultaneously
flowboy --headless             # No UI, run flows from config
```

Default web port: **8042** (overridable with `--port`).

## Key Dependencies

- `github.com/charmbracelet/bubbletea` — TUI framework
- `github.com/charmbracelet/lipgloss` — TUI styling
- `gopkg.in/yaml.v3` — config parsing
- Vanilla JS + Canvas — web UI (no framework)

## Style Reference

CodePen "Fallout 4 Pip-Boy in CSS" by Stix (https://codepen.io/stix/pen/KdJEwB)

Used for:
- Color palette (#8df776 green, #000 black, #272b2a panels, #d8c99e accent)
- Droid Sans font, uppercase, bold
- Scan line animation
- Screen reflection overlay
- CRT bezel aesthetic

NOT used for:
- Layout (we use a full dashboard, not compact Pip-Boy screen)
- Hardware elements (no screws, wheels, speakers — just the CRT bezel)

### CodePen CSS Reference (Key Styles to Adapt)

```css
/* Colors & Font */
font-family: 'Droid Sans', sans-serif;
font-size: 7pt;
color: #d8c99e;
font-weight: 700;
text-transform: uppercase;

/* Screen */
background: #272b2a;  /* panel background */
background: #000;     /* screen background */
color: #8df776;       /* green phosphor text */
border: 5px solid #333;

/* Scan line animation */
.scan {
  background: linear-gradient(rgba(0,0,0,0), #7ff12a);
  animation: scan 4s infinite;
}
@keyframes scan {
  0%   { top: -80px; }
  70%  { top: 300px; }
  100% { top: 300px; }
}

/* Screen reflection */
background: linear-gradient(150deg, #fff, rgba(0,0,0,0));
opacity: 0.07;

/* Selection color */
::selection { background: lightgreen; }

/* Nav tabs */
nav span { color: #8df776; }
nav .active {
  border-right: 1px solid #8df776;
  border-left: 1px solid #8df776;
  border-bottom: 1px solid #000;
}

/* Bezel label */
font-family: 'Courier New', sans-serif;
letter-spacing: 2px;
font-size: 12pt;
```
