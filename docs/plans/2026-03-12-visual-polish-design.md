# Visual Polish Design — Bezel Chrome, Globe, Panel HUD

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add three visual enhancement layers to the Pip-Boy themed web UI — bezel chrome framing the screen, a wireframe globe in the status bar, and HUD-style panel decorations.

**Architecture:** Pure CSS for bezel and panel HUD (pseudo-elements, box-shadow, gradients). New JS file for the wireframe globe (canvas 2D, Y-axis rotation). Minimal HTML changes (one canvas element added to status bar).

**Tech Stack:** CSS pseudo-elements, CSS box-shadow/gradients, Canvas 2D API, vanilla JS

---

## Layer A: Medium Bezel Chrome

Creates the "screen of a device" feel — the UI looks like it's behind glass in a metal housing.

**Elements:**
- **Corner bolts:** 4px circles at each corner of `.screen`, positioned via `::before`/`::after` on a wrapper. Color: `#555` with subtle highlight.
- **Side ridges:** 1px horizontal grooves in the bezel padding area (left/right edges). Achieved with repeating-linear-gradient on `.bezel` padding.
- **Bezel underline:** Double-line ridge under the bezel header/label area. `border-bottom` with ridge style or stacked borders.
- **Recessed screen border:** Double-border effect on `.screen` — outer border dark (`#1a1a1a`), inner border dim green (`rgba(141, 247, 118, 0.15)`). Uses `outline` + `border` or `box-shadow` inset.

**Files to modify:**
- `internal/web/static/css/pipboy.css` — bezel and screen styles

## Layer B: Wireframe Globe (Status Bar)

A slowly rotating wireframe globe in the status bar. Spins when the engine is running, stops and dims when off.

**Elements:**
- **Canvas:** 28x28px canvas element in the status bar, left side
- **Rendering:** 3D wireframe sphere projected to 2D. ~3 latitude lines, ~4 longitude lines. Y-axis rotation.
- **Animation:** `requestAnimationFrame` loop. ~8 seconds per full rotation. Stroke color `#8df776`.
- **Engine state:** When engine is off, globe stops rotating and stroke color dims to `#333`.

**Files to create:**
- `internal/web/static/js/globe.js` — ~60 lines, globe rendering and animation

**Files to modify:**
- `internal/web/static/index.html` — add `<canvas id="globe" width="28" height="28">` to status bar
- `internal/web/static/js/app.js` — import/init globe, pass engine state on toggle

## Layer C: Panel HUD Styling

Military/sci-fi HUD decorations on each panel — brackets, glow, status dots, gradient tints.

**Elements:**
- **Corner brackets:** `::before` and `::after` on `.panel` — 8px arms, 1px solid `#8df776` with subtle glow (`box-shadow`). Top-left and bottom-right corners (or all four via nested pseudo-elements).
- **Glowing panel borders:** Replace flat `border: 1px solid #333` with `box-shadow: 0 0 3px rgba(141, 247, 118, 0.15), inset 0 0 3px rgba(141, 247, 118, 0.05)`.
- **Status dots:** 6px circles before panel header text via `::before` on `.panel-header`. Color `#8df776`.
- **Gradient backgrounds:** Panels get subtle gradient from dark green tint at top (`rgba(141, 247, 118, 0.03)`) to pure black at bottom.

**Files to modify:**
- `internal/web/static/css/app.css` — panel, panel-header styles
