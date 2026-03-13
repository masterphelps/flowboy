// Flowboy 3000 - Network Topology Map
// ─────────────────────────────────────────────────────────────────────────────
// Segments distributed above/below a shared "PHYSICAL NETWORK" backbone.
// Fits in viewport; scrolls only if content overflows.
// ─────────────────────────────────────────────────────────────────────────────

class NetworkMap {
    constructor(canvasId) {
        this.canvas = document.getElementById(canvasId);
        if (!this.canvas) return;
        this.ctx = this.canvas.getContext('2d');
        this.viewport = document.getElementById('map-viewport');

        this.machines = [];
        this.flows = [];
        this.segments = [];
        this.flowDots = [];
        this.topSegs = [];
        this.botSegs = [];

        this.selectedMachine = null;
        this.selectedFlow = null;
        this.hoveredElement = null;
        this._animating = false;
        this._backboneY = 0;
        this._backboneX1 = 0;
        this._backboneX2 = 0;
        this._contentW = 0;
        this._contentH = 0;

        this.colors = {
            green:       '#8df776',
            dimGreen:    '#2a5e1e',
            brightGreen: '#7ff12a',
            black:       '#000000',
            accent:      '#d8c99e',
            border:      '#333333',
            darkGreen:   '#1a3816',
            tube:        '#1e4a16',
            tubeBorder:  '#3a7a2e',
            tubeActive:  '#2a6e20',
            muted:       '#5a9a4e',
        };

        this.setupResize();
        this.setupInteraction();
    }

    async _fetch(method, path) {
        try {
            const res = await fetch(path, { method, headers: { 'Content-Type': 'application/json' } });
            if (res.status === 204) return null;
            return await res.json();
        } catch (e) { return null; }
    }

    // ── Resize ───────────────────────────────────────────────────────────
    setupResize() {
        window.addEventListener('resize', () => this.sizeCanvas());
    }

    sizeCanvas() {
        if (!this.viewport || !this.canvas) return;
        this.layout();
        const vw = this.viewport.clientWidth;
        const vh = this.viewport.clientHeight;
        this.canvas.width = Math.max(vw, this._contentW || 0);
        this.canvas.height = Math.max(vh, this._contentH || 0);
    }

    // ── Layout ───────────────────────────────────────────────────────────
    layout() {
        if (!this.segments.length) {
            this._contentW = 0;
            this._contentH = 0;
            return;
        }

        const pad = 30;
        const machineW = 130;
        const segGap = 60;

        // Calculate segment widths first
        for (const seg of this.segments) {
            const mc = seg.machines ? seg.machines.length : 0;
            seg._width = Math.max(150, mc * machineW + 30);
        }

        // ── Single segment: no backbone needed ──
        if (this.segments.length === 1) {
            return this.layoutSingle(pad);
        }

        // ── Multiple segments: distribute above/below backbone ──
        // Greedy balance by width
        this.topSegs = [];
        this.botSegs = [];
        const sorted = [...this.segments].sort((a, b) => b._width - a._width);
        let topW = 0, botW = 0;
        for (const seg of sorted) {
            if (topW <= botW) { this.topSegs.push(seg); topW += seg._width + segGap; }
            else              { this.botSegs.push(seg); botW += seg._width + segGap; }
        }

        // Row widths (including padding)
        const rowW = (segs) => {
            let w = pad * 2;
            for (const s of segs) w += s._width;
            w += Math.max(0, segs.length - 1) * segGap;
            return w;
        };
        const topRowW = rowW(this.topSegs);
        const botRowW = rowW(this.botSegs);
        const contentW = Math.max(topRowW, botRowW);

        // Vertical geometry
        const iconBlock = 70;   // icon center(20) + labels(48) + gap(2)
        const connH = 18;
        const dropH = 28;
        const bbH = 10;         // backbone tube + label

        const topSectionH = iconBlock + connH + dropH;  // machines → tube → drop
        const botSectionH = this.botSegs.length > 0 ? (dropH + connH + iconBlock) : 0;
        const contentH = pad + topSectionH + bbH + botSectionH + pad;
        const backboneY = pad + topSectionH + bbH / 2;

        // Place top segments
        const topTubeY = backboneY - dropH - 4;
        this.placeRow(this.topSegs, topTubeY, 'top', pad, contentW, topRowW, segGap, iconBlock, connH);

        // Place bottom segments
        if (this.botSegs.length > 0) {
            const botTubeY = backboneY + dropH + 4;
            this.placeRow(this.botSegs, botTubeY, 'bottom', pad, contentW, botRowW, segGap, iconBlock, connH);
        }

        // Backbone spans widest row
        const allSegs = [...this.topSegs, ...this.botSegs];
        let minX = Infinity, maxX = 0;
        for (const s of allSegs) {
            if (s._dropX - 20 < minX) minX = s._dropX - 20;
            if (s._dropX + 20 > maxX) maxX = s._dropX + 20;
        }
        this._backboneY = backboneY;
        this._backboneX1 = Math.min(minX, pad);
        this._backboneX2 = Math.max(maxX, contentW - pad);
        this._contentW = contentW;
        this._contentH = contentH;
    }

    layoutSingle(pad) {
        const seg = this.segments[0];
        const mc = seg.machines ? seg.machines.length : 0;
        const iconBlock = 70;
        const connH = 18;

        seg._side = 'top';
        seg._tubeY = pad + iconBlock + connH;
        seg._tubeX1 = pad;
        seg._tubeX2 = pad + seg._width;
        seg._dropX = pad + seg._width / 2;

        if (mc > 0) {
            const step = seg._width / (mc + 1);
            for (let i = 0; i < mc; i++) {
                seg.machines[i].pos = {
                    x: pad + step * (i + 1),
                    y: pad + 20,
                };
            }
        }

        this.topSegs = [seg];
        this.botSegs = [];
        this._backboneY = 0;
        this._backboneX1 = 0;
        this._backboneX2 = 0;
        this._contentW = seg._width + pad * 2;
        this._contentH = seg._tubeY + 25 + pad;
    }

    placeRow(segs, tubeY, side, pad, contentW, rowW, segGap, iconBlock, connH) {
        let x = pad + Math.max(0, (contentW - rowW) / 2);

        for (const seg of segs) {
            seg._side = side;
            seg._tubeY = tubeY;
            seg._tubeX1 = x;
            seg._tubeX2 = x + seg._width;
            seg._dropX = x + seg._width / 2;

            const mc = seg.machines ? seg.machines.length : 0;
            if (mc > 0) {
                const step = seg._width / (mc + 1);
                for (let i = 0; i < mc; i++) {
                    if (side === 'top') {
                        // Machines above tube
                        seg.machines[i].pos = {
                            x: x + step * (i + 1),
                            y: tubeY - connH - iconBlock + 20,
                        };
                    } else {
                        // Machines below tube
                        seg.machines[i].pos = {
                            x: x + step * (i + 1),
                            y: tubeY + connH + 20 + 10,
                        };
                    }
                }
            }

            x += seg._width + segGap;
        }
    }

    // ── Drawing helpers ──────────────────────────────────────────────────
    drawTube(x1, y, x2, active) {
        const h = 8, r = h / 2, ctx = this.ctx;
        ctx.fillStyle = active ? this.colors.tubeActive : this.colors.tube;
        ctx.beginPath(); ctx.roundRect(x1, y - r, x2 - x1, h, r); ctx.fill();
        ctx.strokeStyle = active ? this.colors.green : this.colors.dimGreen;
        ctx.lineWidth = 1;
        ctx.beginPath(); ctx.roundRect(x1, y - r, x2 - x1, h, r); ctx.stroke();
    }

    drawMachineIcon(cx, cy, color, scale) {
        const s = scale || 3, ctx = this.ctx;
        const ox = cx - 4 * s, oy = cy - 5 * s;
        ctx.fillStyle = this.colors.darkGreen;
        for (let py = 1; py <= 6; py++)
            for (let px = 1; px <= 6; px++)
                ctx.fillRect(ox + px * s, oy + py * s, s, s);
        ctx.fillStyle = color;
        for (const [px, py] of [
            [0,0],[1,0],[2,0],[3,0],[4,0],[5,0],[6,0],[7,0],
            [0,7],[1,7],[2,7],[3,7],[4,7],[5,7],[6,7],[7,7],
            [0,1],[0,2],[0,3],[0,4],[0,5],[0,6],
            [7,1],[7,2],[7,3],[7,4],[7,5],[7,6],
            [2,3],[3,3],[5,3],[2,5],[3,5],[4,5],
            [3,8],[4,8],[2,9],[3,9],[4,9],[5,9],
        ]) ctx.fillRect(ox + px * s, oy + py * s, s, s);
    }

    drawLabel(text, x, y, color, size, align) {
        this.ctx.fillStyle = color;
        this.ctx.font = `${size || 12}px 'Share Tech Mono', monospace`;
        this.ctx.textAlign = align || 'center';
        this.ctx.textBaseline = 'top';
        this.ctx.fillText(text, x, y);
    }

    drawLine(x1, y1, x2, y2, color, dashed) {
        const ctx = this.ctx;
        ctx.strokeStyle = color; ctx.lineWidth = 1;
        ctx.setLineDash(dashed ? [4, 4] : []);
        ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2); ctx.stroke();
        ctx.setLineDash([]);
    }

    // ── Render: segments ─────────────────────────────────────────────────
    renderSegments() {
        for (const seg of this.segments) {
            if (seg._tubeY == null) continue;
            const hasActive = this.flows.some(f =>
                f.enabled && this.machines.some(m =>
                    m.segment === seg.cidr && (m.name === f.source || m.name === f.destination)
                )
            );
            this.drawTube(seg._tubeX1, seg._tubeY, seg._tubeX2, hasActive);

            // CIDR label below tube
            const cx = (seg._tubeX1 + seg._tubeX2) / 2;
            const labelY = seg._side === 'top' ? seg._tubeY + 6 : seg._tubeY - 16;
            this.drawLabel(seg.cidr || '?', cx, labelY, this.colors.accent, 11, 'center');
        }
    }

    // ── Render: machines ─────────────────────────────────────────────────
    renderMachines() {
        for (const m of this.machines) {
            if (!m.pos) continue;
            let color = this.colors.green;
            if (this.selectedMachine === m.name) color = this.colors.accent;
            else if (this.hoveredElement === m.name) color = this.colors.brightGreen;

            this.drawMachineIcon(m.pos.x, m.pos.y, color, 3);
            this.drawLabel(m.name, m.pos.x, m.pos.y + 20, color, 12, 'center');

            const maskStr = m.mask > 0 ? `${m.ip}/${m.mask}` : m.ip;
            this.drawLabel(maskStr, m.pos.x, m.pos.y + 34, this.colors.muted, 10, 'center');

            // Connection line to tube
            const seg = this.segments.find(s => s.cidr === m.segment);
            if (seg && seg._tubeY != null) {
                if (seg._side === 'top') {
                    this.drawLine(m.pos.x, m.pos.y + 48, m.pos.x, seg._tubeY - 5, this.colors.dimGreen);
                } else {
                    this.drawLine(m.pos.x, m.pos.y - 18, m.pos.x, seg._tubeY + 5, this.colors.dimGreen);
                }
            }
        }
    }

    // ── Render: drop lines to backbone ───────────────────────────────────
    renderDropLines() {
        if (!this._backboneY) return;
        for (const seg of this.segments) {
            if (seg._tubeY == null) continue;
            const hasCross = this.flows.some(f => {
                if (!f.enabled) return false;
                const ss = this.machines.find(m => m.name === f.source);
                const ds = this.machines.find(m => m.name === f.destination);
                return ss && ds && ss.segment !== ds.segment &&
                    (ss.segment === seg.cidr || ds.segment === seg.cidr);
            });
            const color = hasCross ? this.colors.dimGreen : this.colors.border;
            if (seg._side === 'top') {
                this.drawLine(seg._dropX, seg._tubeY + 5, seg._dropX, this._backboneY - 5, color, !hasCross);
            } else {
                this.drawLine(seg._dropX, this._backboneY + 5, seg._dropX, seg._tubeY - 5, color, !hasCross);
            }
        }
    }

    // ── Render: backbone ─────────────────────────────────────────────────
    renderBackbone() {
        if (!this._backboneY || this.segments.length < 2) return;
        const hasAny = this.flows.some(f => {
            if (!f.enabled) return false;
            const ss = this.machines.find(m => m.name === f.source);
            const ds = this.machines.find(m => m.name === f.destination);
            return ss && ds && ss.segment !== ds.segment;
        });
        this.drawTube(this._backboneX1, this._backboneY, this._backboneX2, hasAny);
        const cx = (this._backboneX1 + this._backboneX2) / 2;
        this.drawLabel('PHYSICAL NETWORK', cx, this._backboneY + 8, this.colors.accent, 11, 'center');
    }

    // ── Flow paths ───────────────────────────────────────────────────────
    computeFlowPath(flow) {
        const src = this.machines.find(m => m.name === flow.source);
        const dst = this.machines.find(m => m.name === flow.destination);
        if (!src || !dst || !src.pos || !dst.pos) return null;
        const srcSeg = this.segments.find(s => s.cidr === src.segment);
        const dstSeg = this.segments.find(s => s.cidr === dst.segment);
        if (!srcSeg || !dstSeg || srcSeg._tubeY == null || dstSeg._tubeY == null) return null;

        if (srcSeg === dstSeg) {
            return [
                { x: src.pos.x, y: srcSeg._tubeY },
                { x: dst.pos.x, y: dstSeg._tubeY },
            ];
        }
        if (!this._backboneY) return null;
        return [
            { x: src.pos.x, y: srcSeg._tubeY },
            { x: srcSeg._dropX, y: srcSeg._tubeY },
            { x: srcSeg._dropX, y: this._backboneY },
            { x: dstSeg._dropX, y: this._backboneY },
            { x: dstSeg._dropX, y: dstSeg._tubeY },
            { x: dst.pos.x, y: dstSeg._tubeY },
        ];
    }

    // ── Flow dot animation ───────────────────────────────────────────────
    updateFlowDots() {
        for (let i = this.flowDots.length - 1; i >= 0; i--) {
            this.flowDots[i].progress += this.flowDots[i].speed;
            if (this.flowDots[i].progress >= 1.0) this.flowDots.splice(i, 1);
        }
        for (const flow of this.flows) {
            if (!flow.enabled) continue;
            const path = this.computeFlowPath(flow);
            if (!path) continue;
            flow._path = path;
            flow._tick = (flow._tick || 0) + 1;
            if (flow._tick >= 180) {
                flow._tick = 0;
                this.flowDots.push({ path, progress: 0, speed: 0.003 + Math.random() * 0.001 });
            }
        }
    }

    getPathPos(path, t) {
        if (!path || path.length < 2) return { x: 0, y: 0 };
        let total = 0;
        const lens = [];
        for (let i = 0; i < path.length - 1; i++) {
            const dx = path[i + 1].x - path[i].x, dy = path[i + 1].y - path[i].y;
            lens.push(Math.sqrt(dx * dx + dy * dy));
            total += lens[i];
        }
        if (total === 0) return path[0];
        let dist = t * total;
        for (let i = 0; i < lens.length; i++) {
            if (dist <= lens[i] || i === lens.length - 1) {
                const frac = lens[i] > 0 ? dist / lens[i] : 0;
                return {
                    x: path[i].x + (path[i + 1].x - path[i].x) * frac,
                    y: path[i].y + (path[i + 1].y - path[i].y) * frac,
                };
            }
            dist -= lens[i];
        }
        return path[path.length - 1];
    }

    renderFlowDots() {
        const ctx = this.ctx;
        for (const dot of this.flowDots) {
            const pos = this.getPathPos(dot.path, dot.progress);
            ctx.beginPath(); ctx.arc(pos.x, pos.y, 6, 0, Math.PI * 2);
            ctx.fillStyle = 'rgba(141, 247, 118, 0.12)'; ctx.fill();
            ctx.beginPath(); ctx.arc(pos.x, pos.y, 3, 0, Math.PI * 2);
            ctx.fillStyle = this.colors.brightGreen; ctx.fill();
        }
    }

    // ── Highlights ───────────────────────────────────────────────────────
    renderHighlights() {
        if (!this.selectedMachine) return;
        const m = this.machines.find(mm => mm.name === this.selectedMachine);
        if (!m || !m.pos) return;
        const pulse = Math.sin(Date.now() / 500) * 0.3 + 0.7;
        this.ctx.strokeStyle = `rgba(216, 201, 158, ${pulse})`;
        this.ctx.lineWidth = 2;
        this.ctx.strokeRect(m.pos.x - 16, m.pos.y - 18, 32, 70);
    }

    // ── Hit testing ──────────────────────────────────────────────────────
    hitTestMachine(cx, cy) {
        for (const m of this.machines) {
            if (!m.pos) continue;
            if (Math.abs(cx - m.pos.x) < 22 && Math.abs(cy - m.pos.y) < 28) return m;
        }
        return null;
    }

    // ── Empty state ──────────────────────────────────────────────────────
    drawEmptyState() {
        const cx = this.canvas.width / 2, cy = this.canvas.height / 2;
        this.drawMachineIcon(cx, cy - 20, this.colors.border, 4);
        this.drawLabel('NO MACHINES', cx, cy + 30, this.colors.border, 16, 'center');
        this.drawLabel('ADD MACHINES TO SEE NETWORK MAP', cx, cy + 50, this.colors.dimGreen, 12, 'center');
    }

    // ── Main render loop ─────────────────────────────────────────────────
    render() {
        if (!this.canvas) return;
        this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);

        if (!this.machines.length) {
            this.drawEmptyState();
        } else {
            this.renderBackbone();
            this.renderDropLines();
            this.renderSegments();
            this.renderMachines();
            this.renderFlowDots();
            this.renderHighlights();
        }
        this.updateFlowDots();
        requestAnimationFrame(() => this.render());
    }

    // ── Interaction ──────────────────────────────────────────────────────
    setupInteraction() {
        this.canvas.addEventListener('click', (e) => {
            const rect = this.canvas.getBoundingClientRect();
            const x = e.clientX - rect.left, y = e.clientY - rect.top;
            const machine = this.hitTestMachine(x, y);
            if (machine) {
                this.selectedMachine = this.selectedMachine === machine.name ? null : machine.name;
                this.selectedFlow = null;
                window.dispatchEvent(new CustomEvent('map-machine-selected', { detail: { name: this.selectedMachine } }));
                return;
            }
            this.selectedMachine = null;
            this.selectedFlow = null;
        });

        this.canvas.addEventListener('mousemove', (e) => {
            const rect = this.canvas.getBoundingClientRect();
            const x = e.clientX - rect.left, y = e.clientY - rect.top;
            const machine = this.hitTestMachine(x, y);
            this.hoveredElement = machine ? machine.name : null;
            this.canvas.style.cursor = machine ? 'pointer' : 'default';
        });

        this.canvas.addEventListener('mouseleave', () => {
            this.hoveredElement = null;
            this.canvas.style.cursor = 'default';
        });

        window.addEventListener('machine-selected', (e) => {
            this.selectedMachine = e.detail ? e.detail.name : null;
        });
        window.addEventListener('flow-selected', (e) => {
            this.selectedFlow = e.detail ? e.detail.name : null;
        });
    }

    // ── Data loading ─────────────────────────────────────────────────────
    async loadData() {
        const [segments, flows] = await Promise.all([
            this._fetch('GET', '/api/segments'),
            this._fetch('GET', '/api/flows'),
        ]);
        this.segments = (segments || []).slice();
        this.flows = flows || [];
        this.segments.sort((a, b) => (a.cidr || '').localeCompare(b.cidr || ''));

        this.machines = [];
        for (const seg of this.segments) {
            if (!seg.machines) continue;
            for (const m of seg.machines) {
                m.segment = seg.cidr;
                m.pos = null;
                this.machines.push(m);
            }
        }
        this.sizeCanvas();
    }

    async start() {
        await this.loadData();
        if (!this._animating) { this._animating = true; this.render(); }
        this._refreshInterval = setInterval(() => this.loadData(), 5000);
    }

    stop() {
        this._animating = false;
        if (this._refreshInterval) { clearInterval(this._refreshInterval); this._refreshInterval = null; }
    }
}

const networkMap = new NetworkMap('network-map');
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => networkMap.start());
} else {
    networkMap.start();
}
