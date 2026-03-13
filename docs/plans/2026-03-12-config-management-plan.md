# Config File Management Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Open/Save/Save As config file management via a FILE drawer in the status bar, and mirror the status bar layout (buttons left, readouts + globe right).

**Architecture:** Four new API endpoints serve config file listing and manipulation from the `configs/` directory. The frontend adds a FILE drawer (matching the existing collector drawer pattern) and reorganizes the status bar. On "open", the engine stops, config reloads into the engine, and all panels refresh.

**Tech Stack:** Go (net/http, os, path/filepath, gopkg.in/yaml.v3), vanilla JS, CSS

---

### Task 1: Backend — Config Management Endpoints

**Files:**
- Modify: `internal/web/server.go:137-158` (routes function)
- Modify: `internal/web/handlers.go` (add new handler section after Segments)

**Step 1: Add routes for config endpoints**

In `internal/web/server.go`, add these lines inside the `routes()` function, after the segments route (line 151) and before the websocket route (line 153):

```go
	s.mux.HandleFunc("/api/configs", s.cors(s.handleConfigs))
	s.mux.HandleFunc("/api/configs/", s.cors(s.handleConfigAction))
```

Also add `"path/filepath"` and `"os"` to the imports in `server.go`.

**Step 2: Add config handlers to handlers.go**

Add this section at the end of `handlers.go`, before the WebSocket section (before line 538 `// ---------- WebSocket ----------`):

```go
// ---------- Configs ----------

func (s *Server) handleConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.listConfigs(w, r)
}

func (s *Server) handleConfigAction(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/api/configs/")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "save":
		s.saveCurrentConfig(w, r)
	case "save-as":
		s.saveConfigAs(w, r)
	case "open":
		s.openConfig(w, r)
	default:
		http.Error(w, "unknown action", http.StatusNotFound)
	}
}

type configListResponse struct {
	Configs []string `json:"configs"`
	Current string   `json:"current"`
}

func (s *Server) listConfigs(w http.ResponseWriter, _ *http.Request) {
	dir := filepath.Dir(s.configPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "cannot read configs directory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			names = append(names, e.Name())
		}
	}
	current := filepath.Base(s.configPath)
	writeJSON(w, http.StatusOK, configListResponse{Configs: names, Current: current})
}

func (s *Server) saveCurrentConfig(w http.ResponseWriter, _ *http.Request) {
	if err := s.saveConfig(); err != nil {
		http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "file": filepath.Base(s.configPath)})
}

func (s *Server) saveConfigAs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	// Sanitize: strip path separators, ensure .yaml extension
	name := filepath.Base(req.Name)
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		name = name + ".yaml"
	}
	dir := filepath.Dir(s.configPath)
	newPath := filepath.Join(dir, name)
	if err := config.SaveConfig(s.config, newPath); err != nil {
		http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.configPath = newPath
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "file": name})
}

func (s *Server) openConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	dir := filepath.Dir(s.configPath)
	newPath := filepath.Join(dir, filepath.Base(req.Name))

	newCfg, err := config.LoadConfig(newPath)
	if err != nil {
		http.Error(w, "load failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Stop engine before swapping config
	s.engine.Stop()

	// Remove all existing flows and machines from engine
	for _, f := range s.engine.Flows() {
		_ = s.engine.RemoveFlow(f.Name)
	}
	for _, m := range s.engine.Machines() {
		s.engine.RemoveMachine(m.Name)
	}

	// Load new machines
	for _, mc := range newCfg.Machines {
		m, err := mc.ToMachine()
		if err != nil {
			continue
		}
		s.engine.AddMachine(m)
	}

	// Load new flows
	for _, fc := range newCfg.Flows {
		f, err := fc.ToFlow()
		if err != nil {
			continue
		}
		_ = s.engine.AddFlow(f)
	}

	// Swap config and path
	*s.config = *newCfg
	s.configPath = newPath

	writeJSON(w, http.StatusOK, map[string]string{"status": "loaded", "file": filepath.Base(newPath)})
}
```

**Step 3: Add imports to handlers.go**

Add `"path/filepath"` and `"os"` to the import block in `handlers.go`.

**Step 4: Verify it compiles**

Run: `go build -o flowboy ./cmd/flowboy`
Expected: Compiles with no errors.

**Step 5: Commit**

```bash
git add internal/web/server.go internal/web/handlers.go
git commit -m "feat: add config list/save/save-as/open API endpoints"
```

---

### Task 2: Frontend — Mirror Status Bar Layout

Reorganize the status bar so action buttons (FILE, COLLECTORS) are on the left, and readouts (throughput, flows, pkt/s, engine) + globe are on the right.

**Files:**
- Modify: `internal/web/static/index.html:70-79` (status bar)
- Modify: `internal/web/static/css/app.css:165-182` (status bar + globe styles)

**Step 1: Update status bar HTML**

Replace the entire status bar section (lines 71-79) in `index.html` with:

```html
            <!-- Status bar -->
            <div class="status-bar">
                <button class="status-btn" id="file-btn" onclick="toggleFileDrawer()">FILE</button>
                <button class="status-btn" id="collector-btn" onclick="toggleCollectorDrawer()">COLLECTORS <span id="collector-count">0</span></button>
                <span class="status-spacer"></span>
                <span id="total-throughput">&#8593;0bps</span>
                <span id="active-flows">&#9679;0 flows</span>
                <span id="packets-sec">&#8593;0 pkt/s</span>
                <span id="engine-status">ENGINE: OFF</span>
                <canvas id="globe" width="24" height="24"></canvas>
            </div>
```

**Step 2: Verify layout visually**

Run: `go build -o flowboy ./cmd/flowboy && ./flowboy -web`
Open: `http://localhost:8042`
Expected: FILE and COLLECTORS buttons on left, stats and globe on right.

**Step 3: Commit**

```bash
git add internal/web/static/index.html
git commit -m "feat: mirror status bar layout — buttons left, readouts right"
```

---

### Task 3: Frontend — File Drawer HTML, CSS, and JS

Add the file drawer that slides up from the status bar, matching the collector drawer pattern.

**Files:**
- Modify: `internal/web/static/index.html:58-68` (add file drawer before collector drawer)
- Modify: `internal/web/static/css/app.css` (add file drawer styles)
- Modify: `internal/web/static/js/app.js` (add file drawer logic)

**Step 1: Add file drawer HTML**

In `index.html`, add this block immediately before the collector drawer (`<div class="collector-drawer"...>`):

```html
            <!-- File drawer (slides up from status bar) -->
            <div class="file-drawer" id="file-drawer">
                <div class="drawer-header">
                    <span>FILE — <span id="current-config-name">loading...</span></span>
                    <button class="drawer-close" onclick="toggleFileDrawer()">&times;</button>
                </div>
                <div class="drawer-content" id="config-list"></div>
                <div class="drawer-footer">
                    <button onclick="saveConfig()">SAVE</button>
                    <input type="text" id="save-as-name" placeholder="config name" style="flex:1; margin:0;">
                    <button onclick="saveConfigAs()">SAVE AS</button>
                </div>
            </div>
```

**Step 2: Add file drawer CSS**

Add these styles to `app.css` after the collector drawer section (after line 529):

```css
/* ──── File Drawer ──── */
.file-drawer {
    position: absolute;
    bottom: 30px;
    left: 0;
    right: 0;
    max-height: 0;
    overflow: hidden;
    background: #0a0a0a;
    border-top: 1px solid #333;
    transition: max-height 0.25s ease;
    z-index: 51;
}

.file-drawer.open {
    max-height: 260px;
    border-top: 1px solid #8df776;
    box-shadow: 0 -2px 8px rgba(141, 247, 118, 0.1);
}

.file-drawer .drawer-footer {
    display: flex;
    gap: 6px;
    align-items: center;
}

.file-drawer .drawer-footer input {
    background: #000;
    border: 1px solid #333;
    color: #8df776;
    font-family: 'Share Tech Mono', monospace;
    font-size: 11px;
    padding: 3px 8px;
    text-transform: uppercase;
}

.file-drawer .drawer-footer input:focus {
    outline: none;
    border-color: #8df776;
    box-shadow: 0 0 4px rgba(141, 247, 118, 0.3);
}

.config-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 0;
    border-bottom: 1px solid rgba(51, 51, 51, 0.3);
    cursor: pointer;
    transition: background 0.15s;
}

.config-row:hover {
    background: rgba(141, 247, 118, 0.05);
}

.config-row.current {
    border-left: 2px solid #8df776;
    background: rgba(141, 247, 118, 0.08);
}

.config-name {
    flex: 1;
    color: #8df776;
    font-size: 12px;
}

.config-row.current .config-name::after {
    content: ' (loaded)';
    color: #5a9a4e;
    font-size: 10px;
}
```

**Step 3: Add file drawer JS to app.js**

Add a new state variable at the top of app.js (after `let waveFrame = 0;`):

```js
let currentConfigName = '';  // currently loaded config filename
```

Add a new section in app.js after the Collector Drawer section (after `deleteCollector` function) and before the Status Bar section:

```js
// ──── File Drawer ────────────────────────────────────────────────────────
async function fetchConfigs() {
    try {
        const data = await api('GET', '/api/configs');
        currentConfigName = data.current || '';
        renderConfigs(data.configs || []);
        const nameEl = document.getElementById('current-config-name');
        if (nameEl) nameEl.textContent = currentConfigName || 'unsaved';
    } catch (e) { /* ignore */ }
}

function renderConfigs(configs) {
    const el = document.getElementById('config-list');
    if (!el) return;

    if (!configs.length) {
        el.innerHTML = '<div class="empty-msg">no config files found</div>';
        return;
    }

    el.innerHTML = configs.map(name => {
        const isCurrent = (name === currentConfigName) ? ' current' : '';
        return `<div class="config-row${isCurrent}" onclick="openConfig('${esc(name)}')">
            <span class="config-name">${esc(name)}</span>
        </div>`;
    }).join('');
}

function toggleFileDrawer() {
    const drawer = document.getElementById('file-drawer');
    const btn = document.getElementById('file-btn');
    if (!drawer) return;
    // Close collector drawer if open
    const collDrawer = document.getElementById('collector-drawer');
    if (collDrawer && collDrawer.classList.contains('open')) {
        collDrawer.classList.remove('open');
        const collBtn = document.getElementById('collector-btn');
        if (collBtn) collBtn.classList.remove('active');
    }
    drawer.classList.toggle('open');
    if (btn) btn.classList.toggle('active', drawer.classList.contains('open'));
    if (drawer.classList.contains('open')) fetchConfigs();
}

async function saveConfig() {
    await api('POST', '/api/configs/save');
    await fetchConfigs();
}

async function saveConfigAs() {
    const nameInput = document.getElementById('save-as-name');
    const name = nameInput ? nameInput.value.trim() : '';
    if (!name) return;
    await api('POST', '/api/configs/save-as', { name });
    if (nameInput) nameInput.value = '';
    await fetchConfigs();
}

async function openConfig(name) {
    if (name === currentConfigName) return;
    if (!confirm(`Load "${name}"? This will stop the engine and replace the current config.`)) return;
    await api('POST', '/api/configs/open', { name });
    // Refresh everything
    await Promise.all([
        fetchMachines(),
        fetchFlows(),
        fetchCollectors(),
        fetchEngineStatus(),
        fetchConfigs(),
    ]);
}
```

**Step 4: Update the `toggleCollectorDrawer` function to close the file drawer when opening**

Replace the existing `toggleCollectorDrawer` function:

```js
function toggleCollectorDrawer() {
    const drawer = document.getElementById('collector-drawer');
    const btn = document.getElementById('collector-btn');
    if (!drawer) return;
    // Close file drawer if open
    const fileDrawer = document.getElementById('file-drawer');
    if (fileDrawer && fileDrawer.classList.contains('open')) {
        fileDrawer.classList.remove('open');
        const fileBtn = document.getElementById('file-btn');
        if (fileBtn) fileBtn.classList.remove('active');
    }
    drawer.classList.toggle('open');
    if (btn) btn.classList.toggle('active', drawer.classList.contains('open'));
}
```

**Step 5: Add `fetchConfigs()` to init**

In the `init()` function, add `fetchConfigs()` to the `Promise.all` array:

```js
    await Promise.all([
        fetchMachines(),
        fetchFlows(),
        fetchCollectors(),
        fetchEngineStatus(),
        fetchConfigs(),
    ]);
```

**Step 6: Build and verify**

Run: `go build -o flowboy ./cmd/flowboy && ./flowboy -web`
Open: `http://localhost:8042`
Expected:
- FILE button in status bar left side
- Clicking FILE opens drawer showing config files
- Current config highlighted with "(loaded)" label
- SAVE button writes to current file
- SAVE AS with name input creates new config
- Clicking a different config opens it (confirms first, stops engine, refreshes panels)
- Only one drawer open at a time (file closes collector, collector closes file)

**Step 7: Commit**

```bash
git add internal/web/static/index.html internal/web/static/css/app.css internal/web/static/js/app.js
git commit -m "feat: add FILE drawer for config open/save/save-as management"
```

---

### Task 4: Build and Verify Full Feature

**Step 1: Full build**

Run: `go build -o flowboy ./cmd/flowboy`
Expected: Compiles cleanly.

**Step 2: Manual test sequence**

Run: `./flowboy -web`
Open: `http://localhost:8042`

Test checklist:
1. Status bar layout: `[ FILE ] [ COLLECTORS 0 ] ···spacer··· ↑0bps ●0 flows ↑0 pkt/s ENGINE: OFF [globe]`
2. Click FILE → drawer opens, shows `flowboy.yaml` as current
3. Click SAVE → no error, drawer stays open
4. Type "test-config" in Save As input, click SAVE AS → new file created
5. Click FILE again → both `flowboy.yaml` and `test-config.yaml` listed, `test-config.yaml` is current
6. Click `flowboy.yaml` → confirm dialog → engine stops, panels refresh with flowboy.yaml data
7. Click COLLECTORS while FILE drawer is open → FILE closes, COLLECTORS opens
8. Click FILE while COLLECTORS drawer is open → COLLECTORS closes, FILE opens

**Step 3: Clean up test config**

Run: `rm configs/test-config.yaml` (if created during testing)

**Step 4: Final commit if any adjustments needed**

```bash
git add -A
git commit -m "fix: adjustments from config management testing"
```
