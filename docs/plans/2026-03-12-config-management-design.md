# Config File Management — Design

**Goal:** Add Open/Save/Save As config management to the Flowboy web UI via a Pip-Boy style drawer.

## Status Bar Reorganization

Mirror the current layout. Left side = action buttons, right side = readouts + globe:

```
[ FILE ] [ COLLECTORS 1 ]  ·····spacer·····  ↑0bps  ●0 flows  ↑0 pkt/s  ENGINE: OFF  [globe]
```

## FILE Drawer

Slide-up drawer (same pattern as collector drawer), triggered by FILE button in status bar. Contains:

- **Header:** "FILE — LOADED: flowboy.yaml"
- **Config list:** All `.yaml` files found in `configs/` directory. Click to open (confirms if engine is running).
- **SAVE button:** Overwrites current config file with in-memory state.
- **SAVE AS:** Text input for name + button. Saves to `configs/<name>.yaml`.

## API Endpoints

| Method | Path | Body | Behavior |
|--------|------|------|----------|
| GET | `/api/configs` | — | Lists all `.yaml` filenames in `configs/` dir, plus which is currently loaded |
| POST | `/api/configs/save` | — | Writes in-memory config to current file path |
| POST | `/api/configs/save-as` | `{"name":"x"}` | Writes to `configs/x.yaml`, updates current path |
| POST | `/api/configs/open` | `{"name":"x"}` | Stops engine, loads `configs/x.yaml`, replaces config |

## Behavior

- **Open:** Stops engine if running, loads new config, refreshes all panels (machines, flows, collectors).
- **Save:** Writes current in-memory config to its current file path (already happens on individual CRUD ops, but this is an explicit full save).
- **Save As:** Writes to new file, current file path updates to the new file. Subsequent saves go to the new file.
- All configs live in `configs/` directory. No arbitrary filesystem access.

## Constraints

- Config dir is `configs/` relative to the binary's working directory.
- Server needs to track current config path (already does via `-config` flag).
- Engine must stop before loading a new config (flows reference machines that may not exist in new config).
