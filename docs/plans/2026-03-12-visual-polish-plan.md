# Visual Polish Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add three visual enhancement layers — bezel chrome, wireframe globe, panel HUD — to the Pip-Boy themed web UI.

**Architecture:** Pure CSS for bezel and panel HUD (pseudo-elements, box-shadow, gradients). New `globe.js` for wireframe globe (Canvas 2D, Y-axis rotation). One HTML change (canvas in status bar). Integration in `app.js` to pass engine state to globe.

**Tech Stack:** CSS pseudo-elements/gradients/box-shadow, Canvas 2D API, vanilla JS

**Important:** Web assets are embedded via Go `embed.FS`. After ANY file change, rebuild the binary:
```bash
cd /Users/masterphelps/Documents/flowmaster && go build -o flowboy cmd/flowboy/main.go
```

---

### Task 1: Bezel Chrome — Recessed Screen Border

Make the screen look recessed into the bezel with a double-border effect: outer dark frame + inner dim green glow.

**Files:**
- Modify: `internal/web/static/css/pipboy.css:49-59` (`.screen` rule)

**Step 1: Add recessed border to `.screen`**

In `internal/web/static/css/pipboy.css`, replace the `.screen` rule with:

```css
/* Screen */
.screen {
    background: #000;
    border: 2px solid #222;
    border-radius: 8px;
    height: calc(100vh - 60px);
    position: relative;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    /* Recessed screen: dark outer frame + dim green inner glow */
    outline: 2px solid #111;
    outline-offset: -4px;
    box-shadow:
        inset 0 0 8px rgba(141, 247, 118, 0.08),
        inset 0 0 2px rgba(141, 247, 118, 0.15),
        0 0 4px rgba(0, 0, 0, 0.8);
}
```

Key changes from current:
- `border` changed from `3px solid #333` to `2px solid #222` (darker, thinner)
- Added `outline: 2px solid #111` with `-4px` offset for double-border effect
- Added `box-shadow` with inset green glow + outer dark shadow

**Step 2: Verify in browser**

Run: `cd /Users/masterphelps/Documents/flowmaster && go build -o flowboy cmd/flowboy/main.go && ./flowboy`

Open browser to `http://localhost:8080`. The screen area should have a visible recessed border with subtle green inner glow. The screen should look like it's set into the bezel housing.

**Step 3: Commit**

```bash
cd /Users/masterphelps/Documents/flowmaster
git add internal/web/static/css/pipboy.css
git commit -m "style: add recessed screen border with green inner glow"
```

---

### Task 2: Bezel Chrome — Corner Bolts & Side Ridges

Add decorative corner bolts at each screen corner and subtle horizontal ridges along the bezel sides.

**Files:**
- Modify: `internal/web/static/css/pipboy.css:28-36` (`.crt-bezel` rule)
- Modify: `internal/web/static/css/pipboy.css:38-47` (`.bezel-label` rule)

**Step 1: Add corner bolts via `::before` and `::after` on `.crt-bezel`**

Append to `internal/web/static/css/pipboy.css` (after the `.bezel-label` rule, before `/* Screen */`):

```css
/* Corner bolts on bezel */
.crt-bezel::before,
.crt-bezel::after {
    content: '';
    position: absolute;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: radial-gradient(circle at 35% 35%, #777, #444 60%, #333);
    box-shadow: 0 0 2px rgba(0, 0, 0, 0.6);
    z-index: 20;
}

.crt-bezel::before {
    top: 10px;
    left: 10px;
    /* Second bolt via box-shadow clone positioned at top-right */
    box-shadow:
        0 0 2px rgba(0, 0, 0, 0.6),
        calc(100vw - 28px) 0 0 0 #555,
        calc(100vw - 28px) 0 2px rgba(0, 0, 0, 0.6);
}

.crt-bezel::after {
    bottom: 10px;
    left: 10px;
    /* Second bolt via box-shadow clone positioned at bottom-right */
    box-shadow:
        0 0 2px rgba(0, 0, 0, 0.6),
        calc(100vw - 28px) 0 0 0 #555,
        calc(100vw - 28px) 0 2px rgba(0, 0, 0, 0.6);
}
```

**Step 2: Add side ridges and bezel underline**

Update the `.crt-bezel` rule — change the `background` to include ridge texture, and add a ridge border under `.bezel-label`:

Replace `.crt-bezel` with:

```css
.crt-bezel {
    width: 100vw;
    height: 100vh;
    background:
        /* Side ridges — subtle grooves on left and right edges */
        repeating-linear-gradient(
            180deg,
            transparent,
            transparent 8px,
            rgba(255, 255, 255, 0.03) 8px,
            rgba(255, 255, 255, 0.03) 9px,
            transparent 9px,
            transparent 12px
        ),
        #1a1a1a;
    border: 3px solid #333;
    border-radius: 12px;
    padding: 8px;
    position: relative;
}
```

Replace `.bezel-label` with:

```css
.bezel-label {
    text-align: center;
    font-family: 'Courier New', monospace;
    font-size: 22px;
    font-weight: 700;
    letter-spacing: 6px;
    color: #d8c99e;
    padding: 8px 0;
    text-shadow: 0 0 6px rgba(216, 201, 158, 0.5);
    /* Ridge underline */
    border-bottom: 1px solid #333;
    margin-bottom: 2px;
    box-shadow: 0 1px 0 #222, 0 2px 0 #333;
}
```

Key additions: `border-bottom`, `margin-bottom`, `box-shadow` creates a double-ridge line under "FLOWBOY 3000".

**Step 3: Verify in browser**

Rebuild and run: `cd /Users/masterphelps/Documents/flowmaster && go build -o flowboy cmd/flowboy/main.go && ./flowboy`

Check:
- 4 small circular bolts visible at each corner of the bezel
- Subtle vertical groove texture visible on bezel background (very subtle, look at edges)
- Double ridge line visible under "FLOWBOY 3000" label

**Step 4: Commit**

```bash
cd /Users/masterphelps/Documents/flowmaster
git add internal/web/static/css/pipboy.css
git commit -m "style: add corner bolts, side ridges, and bezel underline"
```

---

### Task 3: Wireframe Globe — Create `globe.js`

Create a new JS file that renders a rotating wireframe sphere on a small canvas. It exports functions to start/stop rotation and change color based on engine state.

**Files:**
- Create: `internal/web/static/js/globe.js`

**Step 1: Write `globe.js`**

Create `internal/web/static/js/globe.js` with the following content:

```javascript
// Wireframe Globe — rotates in status bar when engine is running
// ─────────────────────────────────────────────────────────────────────────────

const globe = {
    canvas: null,
    ctx: null,
    angle: 0,
    running: false,
    animId: null,
    color: '#333',
};

const GLOBE_R = 11;          // sphere radius in px
const GLOBE_CX = 14;         // canvas center X
const GLOBE_CY = 14;         // canvas center Y
const LAT_COUNT = 5;          // latitude lines (including equator)
const LON_COUNT = 8;          // longitude lines
const ROTATION_SPEED = 0.013; // radians per frame (~8s full rotation at 60fps)

function initGlobe(canvasId) {
    globe.canvas = document.getElementById(canvasId);
    if (!globe.canvas) return;
    globe.ctx = globe.canvas.getContext('2d');
    globe.angle = 0;
    drawGlobe();
}

function setGlobeRunning(isRunning) {
    globe.running = isRunning;
    globe.color = isRunning ? '#8df776' : '#333';
    if (isRunning && !globe.animId) {
        animateGlobe();
    }
    if (!isRunning) {
        drawGlobe(); // draw once in dim state
    }
}

function animateGlobe() {
    if (!globe.running) {
        globe.animId = null;
        return;
    }
    globe.angle += ROTATION_SPEED;
    if (globe.angle > Math.PI * 2) globe.angle -= Math.PI * 2;
    drawGlobe();
    globe.animId = requestAnimationFrame(animateGlobe);
}

function drawGlobe() {
    const ctx = globe.ctx;
    if (!ctx) return;
    const w = globe.canvas.width;
    const h = globe.canvas.height;
    ctx.clearRect(0, 0, w, h);

    ctx.strokeStyle = globe.color;
    ctx.lineWidth = 0.8;

    // Draw outer circle (equator outline)
    ctx.beginPath();
    ctx.arc(GLOBE_CX, GLOBE_CY, GLOBE_R, 0, Math.PI * 2);
    ctx.stroke();

    // Latitude lines — horizontal ellipses
    for (let i = 1; i < LAT_COUNT; i++) {
        const lat = (Math.PI / LAT_COUNT) * i - Math.PI / 2;
        const y = GLOBE_CY + GLOBE_R * Math.sin(lat);
        const rx = GLOBE_R * Math.cos(lat);
        if (rx < 1) continue;
        ctx.beginPath();
        ctx.ellipse(GLOBE_CX, y, rx, rx * 0.15, 0, 0, Math.PI * 2);
        ctx.stroke();
    }

    // Longitude lines — vertical ellipses rotated by globe.angle
    for (let i = 0; i < LON_COUNT; i++) {
        const lon = (Math.PI / LON_COUNT) * i + globe.angle;
        const rx = Math.abs(GLOBE_R * Math.cos(lon));
        if (rx < 0.5) continue;
        ctx.beginPath();
        ctx.ellipse(GLOBE_CX, GLOBE_CY, rx, GLOBE_R, 0, 0, Math.PI * 2);
        ctx.stroke();
    }
}
```

**Step 2: Verify file exists**

Run: `cat internal/web/static/js/globe.js | head -5`

Expected: First 5 lines of the file showing the comment header.

**Step 3: Commit**

```bash
cd /Users/masterphelps/Documents/flowmaster
git add internal/web/static/js/globe.js
git commit -m "feat: add wireframe globe renderer for status bar"
```

---

### Task 4: Wireframe Globe — HTML & App Integration

Add the globe canvas to the status bar HTML. Wire up `app.js` to initialize the globe and update it when engine state changes.

**Files:**
- Modify: `internal/web/static/index.html:61-66` (status bar section)
- Modify: `internal/web/static/index.html:75-77` (script tags)
- Modify: `internal/web/static/js/app.js:214-234` (`updateStatusBar` function)
- Modify: `internal/web/static/js/app.js:492-510` (`init` function)

**Step 1: Add globe canvas to status bar**

In `internal/web/static/index.html`, replace the status bar div (lines 61-66):

```html
            <!-- Status bar -->
            <div class="status-bar">
                <canvas id="globe" width="28" height="28"></canvas>
                <span id="total-throughput">&#8593;0bps</span>
                <span id="active-flows">&#9679;0 flows</span>
                <span id="packets-sec">&#8593;0 pkt/s</span>
                <span id="engine-status">ENGINE: OFF</span>
            </div>
```

Only change: added `<canvas id="globe" width="28" height="28"></canvas>` as the first child.

**Step 2: Add globe script tag**

In `internal/web/static/index.html`, add the globe script between `map.js` and `app.js`:

```html
    <script src="/js/ws.js"></script>
    <script src="/js/map.js"></script>
    <script src="/js/globe.js"></script>
    <script src="/js/app.js"></script>
```

**Step 3: Initialize globe in `app.js` init function**

In `internal/web/static/js/app.js`, in the `init()` function (around line 492), add globe initialization after the WebSocket setup. Replace:

```javascript
async function init() {
    // Connect WebSocket
    flowboyWS.onStats = handleWSStats;
    flowboyWS.connect();
```

With:

```javascript
async function init() {
    // Connect WebSocket
    flowboyWS.onStats = handleWSStats;
    flowboyWS.connect();

    // Initialize wireframe globe
    initGlobe('globe');
```

**Step 4: Update globe state when engine status changes**

In `internal/web/static/js/app.js`, in the `updateStatusBar()` function (around line 233), add globe state update after the engine status text. After this line:

```javascript
    if (esEl) esEl.textContent = 'ENGINE: ' + (engineStatus.running ? 'RUNNING' : 'OFF');
```

Add:

```javascript
    // Update globe animation state
    if (typeof setGlobeRunning === 'function') {
        setGlobeRunning(engineStatus.running);
    }
```

**Step 5: Add globe canvas CSS**

In `internal/web/static/css/app.css`, add after the `.status-bar` rule (line 99):

```css
#globe {
    flex: 0 0 28px;
    width: 28px;
    height: 28px;
}
```

**Step 6: Verify in browser**

Rebuild and run: `cd /Users/masterphelps/Documents/flowmaster && go build -o flowboy cmd/flowboy/main.go && ./flowboy`

Check:
- Small wireframe globe visible at left end of status bar
- Globe is dim (#333) when engine is off
- Start engine → globe turns green (#8df776) and rotates
- Stop engine → globe dims and stops

**Step 7: Commit**

```bash
cd /Users/masterphelps/Documents/flowmaster
git add internal/web/static/index.html internal/web/static/js/app.js internal/web/static/css/app.css
git commit -m "feat: integrate wireframe globe into status bar with engine state"
```

---

### Task 5: Panel HUD — Corner Brackets

Add HUD-style corner brackets to each panel using CSS pseudo-elements. Two corners per panel (top-left via `::before`, bottom-right via `::after`).

**Files:**
- Modify: `internal/web/static/css/app.css:37-46` (`.panel` rule) and add new rules after it

**Step 1: Update `.panel` for positioning context and add bracket pseudo-elements**

In `internal/web/static/css/app.css`, replace the `.panel` rule with:

```css
.panel {
    border: 1px solid rgba(141, 247, 118, 0.2);
    border-radius: 4px;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-height: 0;
    height: 100%;
    position: relative;
    /* Subtle green outer glow */
    box-shadow:
        0 0 4px rgba(141, 247, 118, 0.08),
        inset 0 0 4px rgba(141, 247, 118, 0.03);
}
```

Key changes:
- `border` changed from `#333` to `rgba(141, 247, 118, 0.2)` (dim green)
- Added `position: relative` (needed for absolute bracket positioning)
- Added `box-shadow` for subtle glow

Then append these new rules right after `.panel`:

```css
/* HUD corner brackets — top-left */
.panel::before {
    content: '';
    position: absolute;
    top: -1px;
    left: -1px;
    width: 10px;
    height: 10px;
    border-top: 1px solid #8df776;
    border-left: 1px solid #8df776;
    z-index: 5;
    pointer-events: none;
}

/* HUD corner brackets — bottom-right */
.panel::after {
    content: '';
    position: absolute;
    bottom: -1px;
    right: -1px;
    width: 10px;
    height: 10px;
    border-bottom: 1px solid #8df776;
    border-right: 1px solid #8df776;
    z-index: 5;
    pointer-events: none;
}
```

**Step 2: Verify in browser**

Rebuild and run. Check:
- Each panel (Machines, Collectors, Active Flows, Network Map) has a bright green bracket at top-left and bottom-right corners
- Brackets are 10px arm length, 1px thick, green (#8df776)
- Panel border is now dim green instead of gray

**Step 3: Commit**

```bash
cd /Users/masterphelps/Documents/flowmaster
git add internal/web/static/css/app.css
git commit -m "style: add HUD corner brackets and green glow to panels"
```

---

### Task 6: Panel HUD — Status Dots & Gradient Backgrounds

Add green status dots before panel header text and subtle gradient tint backgrounds on panels.

**Files:**
- Modify: `internal/web/static/css/app.css:48-55` (`.panel-header` rule)
- Modify: `internal/web/static/css/app.css:37-46` (`.panel` rule — add gradient)

**Step 1: Add status dots to panel headers**

In `internal/web/static/css/app.css`, replace the `.panel-header` rule with:

```css
.panel-header {
    padding: 8px 12px 8px 24px;
    border-bottom: 1px solid #8df776;
    color: #d8c99e;
    font-size: 15px;
    font-weight: 700;
    letter-spacing: 3px;
    position: relative;
    /* Glowing bottom border */
    box-shadow: 0 1px 4px rgba(141, 247, 118, 0.15);
}

/* Status dot before header text */
.panel-header::before {
    content: '';
    position: absolute;
    left: 10px;
    top: 50%;
    transform: translateY(-50%);
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #8df776;
    box-shadow: 0 0 4px rgba(141, 247, 118, 0.6);
}
```

Key changes:
- `padding-left` increased from 12px to 24px (makes room for status dot)
- Added `position: relative` for absolute dot positioning
- Added `box-shadow` for glowing bottom border
- Added `::before` pseudo-element for the green status dot

**Step 2: Add gradient background to panels**

In `internal/web/static/css/app.css`, update the `.panel` rule's `background` property. Change:

```css
    background: rgba(0, 0, 0, 0.5);
```

To:

```css
    background: linear-gradient(
        180deg,
        rgba(141, 247, 118, 0.03) 0%,
        rgba(0, 0, 0, 0.5) 30%,
        rgba(0, 0, 0, 0.6) 100%
    );
```

This creates a very subtle green tint at the top of each panel that fades to black.

**Step 3: Verify in browser**

Rebuild and run. Check:
- Each panel header has a small glowing green dot to the left of the text
- Panel header bottom border has a subtle green glow
- Panel backgrounds have a very faint green tint at the top, fading to dark

**Step 4: Commit**

```bash
cd /Users/masterphelps/Documents/flowmaster
git add internal/web/static/css/app.css
git commit -m "style: add status dots to panel headers and gradient backgrounds"
```

---

### Task 7: Build & Full Visual Verification

Final rebuild and visual check of all three layers working together.

**Files:**
- None (verification only)

**Step 1: Full rebuild**

```bash
cd /Users/masterphelps/Documents/flowmaster && go build -o flowboy cmd/flowboy/main.go
```

**Step 2: Launch and verify all enhancements**

Run: `./flowboy`

Open `http://localhost:8080` in browser. Verify the complete checklist:

**Bezel Chrome:**
- [ ] Screen has recessed double-border with green inner glow
- [ ] 4 corner bolts visible at bezel corners (small metallic circles)
- [ ] Subtle ridge texture on bezel background
- [ ] Double-line ridge under "FLOWBOY 3000" label

**Globe:**
- [ ] Wireframe globe visible at left of status bar
- [ ] Globe is dim (#333) when engine off
- [ ] Globe turns green and rotates when engine starts
- [ ] Globe stops and dims when engine stops

**Panel HUD:**
- [ ] Green corner brackets on each panel (top-left and bottom-right)
- [ ] Panels have dim green border with subtle glow
- [ ] Green status dot before each panel header
- [ ] Panel headers have glowing bottom border
- [ ] Panels have subtle green gradient tint at top

**No regressions:**
- [ ] All panels still scroll correctly
- [ ] Status bar visible at bottom
- [ ] Network map renders and scrolls
- [ ] Modals still work (add machine, add flow, etc.)
- [ ] Fonts remain readable (no blur from new effects)

**Step 3: Commit (if any final tweaks needed)**

```bash
cd /Users/masterphelps/Documents/flowmaster
git add -A
git commit -m "style: visual polish complete — bezel chrome, globe, panel HUD"
```
