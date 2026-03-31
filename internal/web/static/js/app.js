// Flowboy 3000 - Main SPA
// ─────────────────────────────────────────────────────────────────────────────

// ──── State ──────────────────────────────────────────────────────────────────
let machines = [];
let flows = [];
let collectors = [];
let engineStatus = { running: false, flow_count: 0, uptime: '0s' };
let selectedMachine = null; // name of currently selected machine
let selectedFlow = null;    // name of currently selected flow
let flowSortBy = 'source';  // 'source' or 'destination' — default to source
let waveFrame = 0;          // animation tick counter
let currentConfigName = '';  // currently loaded config filename
let anomalyScenarios = [];   // available anomaly scenarios
let activeAnomalies = [];    // currently active anomalies
let viewMode = 'dashboard';  // 'dashboard' or 'fullscreen'
let fullscreenTab = 'machines'; // 'machines', 'flows', or 'map'

const WAVE_CHARS = ['~', '\u223f', '~', '\u2248', '\u223f', '\u2248'];

// ──── API Helper ─────────────────────────────────────────────────────────────
async function api(method, path, body) {
    const opts = { method, headers: { 'Content-Type': 'application/json' } };
    if (body) opts.body = JSON.stringify(body);
    const res = await fetch(path, opts);
    if (res.status === 204) return null;
    if (!res.ok) {
        const msg = await res.text();
        throw new Error(msg);
    }
    return res.json();
}

// ──── Data Fetchers ──────────────────────────────────────────────────────────
async function fetchMachines() {
    try { machines = await api('GET', '/api/machines') || []; } catch (e) { machines = []; }
    renderMachines();
}

async function fetchFlows() {
    try { flows = await api('GET', '/api/flows') || []; } catch (e) { flows = []; }
    renderFlows();
}

async function fetchCollectors() {
    try { collectors = await api('GET', '/api/collectors') || []; } catch (e) { collectors = []; }
    renderCollectors();
}

async function fetchEngineStatus() {
    try {
        engineStatus = await api('GET', '/api/engine/status') || engineStatus;
    } catch (e) { /* keep existing */ }
    updateStatusBar();
}

// ──── Fluctuation Control ────────────────────────────────────────────────────
async function fetchFluctuation() {
    try {
        const data = await api('GET', '/api/fluctuation');
        const floorPct = Math.round((data.floor || 1.0) * 100);
        const ceilPct = Math.round((data.ceiling || 1.0) * 100);
        const floorSlider = document.getElementById('fluct-floor');
        const ceilSlider = document.getElementById('fluct-ceil');
        const floorLabel = document.getElementById('fluct-floor-pct');
        const ceilLabel = document.getElementById('fluct-ceil-pct');
        if (floorSlider) floorSlider.value = floorPct;
        if (ceilSlider) ceilSlider.value = ceilPct;
        if (floorLabel) floorLabel.textContent = floorPct + '%';
        if (ceilLabel) ceilLabel.textContent = ceilPct + '%';
    } catch (e) { /* ignore */ }
}

let fluctDebounce = null;
function updateFluctSliders() {
    const floorVal = parseInt(document.getElementById('fluct-floor').value, 10);
    const ceilVal = parseInt(document.getElementById('fluct-ceil').value, 10);
    const floorLabel = document.getElementById('fluct-floor-pct');
    const ceilLabel = document.getElementById('fluct-ceil-pct');
    if (floorLabel) floorLabel.textContent = floorVal + '%';
    if (ceilLabel) ceilLabel.textContent = ceilVal + '%';

    if (fluctDebounce) clearTimeout(fluctDebounce);
    fluctDebounce = setTimeout(async () => {
        try {
            await api('PUT', '/api/fluctuation', {
                floor: floorVal / 100,
                ceiling: ceilVal / 100,
                period: '1h',
            });
        } catch (e) { /* ignore */ }
    }, 200);
}

// ──── Anomaly-Affected Machines ──────────────────────────────────────────────
// Returns a Set of machine names involved in active anomalies.
// If an anomaly has no targets (global), ALL machines are affected.
function getAnomalyAffectedMachines() {
    const affected = new Set();
    if (!activeAnomalies || !activeAnomalies.length) return affected;

    for (const a of activeAnomalies) {
        if (!a.targets || a.targets.length === 0) {
            // Global anomaly — all machines affected
            for (const m of machines) affected.add(m.name);
        } else {
            for (const t of a.targets) affected.add(t);
        }
    }
    return affected;
}

// ──── Machine Panel ──────────────────────────────────────────────────────────
function renderMachines() {
    const el = document.getElementById('machines-list');
    if (!el) return;

    if (!machines.length) {
        el.innerHTML = '<div class="empty-msg">no machines configured</div>';
        if (viewMode === 'fullscreen' && fullscreenTab === 'machines') setFullscreenTab('machines');
        return;
    }

    const sorted = [...machines].sort((a, b) => a.name.localeCompare(b.name));
    const affected = getAnomalyAffectedMachines();
    el.innerHTML = sorted.map(m => {
        const sel = (selectedMachine === m.name) ? ' selected' : '';
        const anomaly = affected.has(m.name) ? ' anomaly-attacker' : '';
        const maskStr = m.mask > 0 ? `/${m.mask}` : '';
        return `<div class="row machine-row${sel}${anomaly}" data-name="${esc(m.name)}" onclick="selectMachine('${esc(m.name)}')">
            <span class="row-label">${esc(m.name)}</span>
            <span class="row-detail">${esc(m.ip)}${maskStr}</span>
            <span class="row-actions">
                <button onclick="event.stopPropagation(); showEditMachineForm('${esc(m.name)}', '${esc(m.ip)}', ${m.mask})" title="Edit">\u270e</button>
                <button onclick="event.stopPropagation(); deleteMachine('${esc(m.name)}')" title="Delete">\u2717</button>
            </span>
        </div>`;
    }).join('');
    if (viewMode === 'fullscreen' && fullscreenTab === 'machines') setFullscreenTab('machines');
}

function selectMachine(name) {
    selectedMachine = (selectedMachine === name) ? null : name;
    renderMachines();
    renderFlows(); // filter flows by machine
    // Dispatch event for network map highlighting
    window.dispatchEvent(new CustomEvent('machine-selected', { detail: { name: selectedMachine } }));
}

async function deleteMachine(name) {
    if (!confirm(`Delete machine "${name}"?`)) return;
    const res = await fetch(`/api/machines/${encodeURIComponent(name)}`, { method: 'DELETE' });
    if (!res.ok) {
        const msg = await res.text();
        alert(msg);
        return;
    }
    if (selectedMachine === name) selectedMachine = null;
    await fetchMachines();
    await fetchFlows();
}

// ──── Flow Panel ─────────────────────────────────────────────────────────────
function renderFlows() {
    const el = document.getElementById('flows-list');
    if (!el) return;

    let visible = flows;
    if (selectedMachine) {
        visible = flows.filter(f => f.source === selectedMachine || f.destination === selectedMachine);
    }

    if (!visible.length) {
        const msg = selectedMachine ? `no flows for ${selectedMachine}` : 'no flows configured';
        el.innerHTML = `<div class="empty-msg">${msg}</div>`;
        return;
    }

    if (flowSortBy) {
        visible = [...visible].sort((a, b) => a[flowSortBy].localeCompare(b[flowSortBy]));
    }

    const affected = getAnomalyAffectedMachines();
    el.innerHTML = visible.map(f => {
        const sel = (selectedFlow === f.name) ? ' selected' : '';
        const active = f.enabled ? ' active' : ' inactive';
        const anomaly = (affected.has(f.source) || affected.has(f.destination)) ? ' anomaly-affected' : '';
        const wave = f.enabled ? waveString() : '\u2500\u2500\u2500';
        const statusIcon = f.enabled ? '\u25a0' : '\u25b6';
        const toggleAction = f.enabled ? 'stop' : 'start';
        const toggleLabel = f.enabled ? 'STOP' : 'START';

        return `<div class="row flow-row${sel}${active}${anomaly}" data-name="${esc(f.name)}" onclick="selectFlow('${esc(f.name)}')">
            <span class="flow-endpoints">${esc(f.source)}:${f.source_port} \u2192 ${esc(f.destination)}:${f.destination_port}</span>
            <span class="flow-proto">${esc(f.protocol)}</span>
            <span class="flow-bar"><span class="flow-bar-fill" style="width:${flowBarWidth(f)}%"></span></span>
            <span class="flow-rate">${esc(f.rate)}</span>
            <span class="flow-wave" data-flow="${esc(f.name)}">${wave}</span>
            <span class="row-actions">
                <button onclick="event.stopPropagation(); toggleFlow('${esc(f.name)}', '${toggleAction}')" title="${toggleLabel}">${statusIcon}</button>
                <button onclick="event.stopPropagation(); showEditFlowForm('${esc(f.name)}')" title="Edit">\u270e</button>
                <button onclick="event.stopPropagation(); deleteFlow('${esc(f.name)}')" title="Delete">\u2717</button>
            </span>
        </div>`;
    }).join('');
    if (viewMode === 'fullscreen' && fullscreenTab === 'flows') setFullscreenTab('flows');
}

function selectFlow(name) {
    selectedFlow = (selectedFlow === name) ? null : name;
    renderFlows();
    const flow = flows.find(f => f.name === name);
    window.dispatchEvent(new CustomEvent('flow-selected', { detail: { name: selectedFlow, flow } }));
}

function setFlowSort(field) {
    flowSortBy = field;
    // Update active button styling
    document.querySelectorAll('.sort-btn').forEach(btn => btn.classList.remove('active'));
    if (flowSortBy) {
        const label = flowSortBy === 'source' ? 'SRC' : 'DST';
        document.querySelectorAll('.sort-btn').forEach(btn => {
            if (btn.textContent === label) btn.classList.add('active');
        });
    }
    renderFlows();
}

async function toggleFlow(name, action) {
    await api('POST', `/api/flows/${encodeURIComponent(name)}/${action}`);
    await fetchFlows();
}

async function deleteFlow(name) {
    if (!confirm(`Delete flow "${name}"?`)) return;
    await api('DELETE', `/api/flows/${encodeURIComponent(name)}`);
    if (selectedFlow === name) selectedFlow = null;
    await fetchFlows();
}

// "Start All" and "Stop All" — called from onclick in HTML
async function startAll() {
    await api('POST', '/api/engine/start');
    for (const f of flows) {
        if (!f.enabled) await api('POST', `/api/flows/${encodeURIComponent(f.name)}/start`);
    }
    await fetchFlows();
    await fetchEngineStatus();
}

async function stopAll() {
    for (const f of flows) {
        if (f.enabled) await api('POST', `/api/flows/${encodeURIComponent(f.name)}/stop`);
    }
    await api('POST', '/api/engine/stop');
    await fetchFlows();
    await fetchEngineStatus();
}

function flowBarWidth(f) {
    if (!f.enabled) return 0;
    // Parse rate string to get a rough percentage (cap at 100)
    const num = parseFloat(f.rate) || 0;
    const unit = (f.rate || '').replace(/[\d.]/g, '').toLowerCase();
    let bps = num;
    if (unit.startsWith('k')) bps = num * 1000;
    else if (unit.startsWith('m')) bps = num * 1000000;
    else if (unit.startsWith('g')) bps = num * 1000000000;
    // Scale: 10Mbps = 100%
    return Math.min(100, (bps / 10000000) * 100);
}

function waveString() {
    const len = 3;
    let s = '';
    for (let i = 0; i < len; i++) {
        s += WAVE_CHARS[(waveFrame + i) % WAVE_CHARS.length];
    }
    return s;
}

// ──── Collector Drawer ──────────────────────────────────────────────────────
function renderCollectors() {
    const el = document.getElementById('collectors-list');
    const countEl = document.getElementById('collector-count');
    if (!el) return;

    if (countEl) countEl.textContent = collectors.length;

    if (!collectors.length) {
        el.innerHTML = '<div class="empty-msg">no collectors configured</div>';
        return;
    }

    el.innerHTML = collectors.map(c => {
        const pktCount = c.packets_sent || 0;
        const hasErrors = (c.errors || 0) > 0;
        const onClass = 'on'; // collectors are always "on" once added
        const statsText = pktCount > 0 ? `${formatCount(pktCount)} pkt` : '';
        const errText = hasErrors ? ` \u26a0${c.errors}` : '';
        return `<div class="collector-row" data-name="${esc(c.name)}">
            <div class="dip-switch ${onClass}" title="Active"></div>
            <span class="collector-name">${esc(c.name)}</span>
            <span class="collector-addr">${esc(c.address)}</span>
            <span class="collector-stats">${statsText}${errText}</span>
            <button class="collector-remove" onclick="showEditCollectorForm('${esc(c.name)}')" title="Edit">\u270e</button>
            <button class="collector-remove" onclick="deleteCollector('${esc(c.name)}')" title="Remove">\u2717</button>
        </div>`;
    }).join('');
}

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

async function deleteCollector(name) {
    if (!confirm(`Delete collector "${name}"?`)) return;
    await api('DELETE', `/api/collectors/${encodeURIComponent(name)}`);
    await fetchCollectors();
}

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

async function newConfig() {
    if (!confirm('Create a blank config? This will stop the engine and clear everything.')) return;
    await api('POST', '/api/configs/new');
    await Promise.all([
        fetchMachines(),
        fetchFlows(),
        fetchCollectors(),
        fetchEngineStatus(),
        fetchConfigs(),
    ]);
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

// ──── View Mode ──────────────────────────────────────────────────────────────
function setViewMode(mode) {
    viewMode = mode;
    const dashView = document.getElementById('dashboard-view');
    const fsView = document.getElementById('fullscreen-view');
    const dashBtn = document.getElementById('view-dashboard');
    const fsBtn = document.getElementById('view-fullscreen');

    if (mode === 'dashboard') {
        // Restore panels to dashboard
        restoreDashboard();
        dashView.style.display = '';
        fsView.style.display = 'none';
        dashBtn.classList.add('active');
        fsBtn.classList.remove('active');
    } else {
        dashView.style.display = 'none';
        fsView.style.display = '';
        dashBtn.classList.remove('active');
        fsBtn.classList.add('active');
        setFullscreenTab(fullscreenTab);
    }
}

function setFullscreenTab(tab) {
    fullscreenTab = tab;
    const content = document.getElementById('fs-content');
    const footer = document.getElementById('fs-footer');
    if (!content || !footer) return;

    // Update tab highlights
    document.querySelectorAll('.fs-tab').forEach(t => {
        t.classList.toggle('active', t.dataset.tab === tab);
    });

    // Render content based on tab
    if (tab === 'machines') {
        const sorted = [...machines].sort((a, b) => a.name.localeCompare(b.name));
        const affected = getAnomalyAffectedMachines();
        content.innerHTML = sorted.length ? sorted.map(m => {
            const sel = (selectedMachine === m.name) ? ' selected' : '';
            const anomaly = affected.has(m.name) ? ' anomaly-attacker' : '';
            const maskStr = m.mask > 0 ? `/${m.mask}` : '';
            return `<div class="row machine-row${sel}${anomaly}" data-name="${esc(m.name)}" onclick="selectMachine('${esc(m.name)}')">
                <span class="row-label">${esc(m.name)}</span>
                <span class="row-detail">${esc(m.ip)}${maskStr}</span>
                <span class="row-actions">
                    <button onclick="event.stopPropagation(); showEditMachineForm('${esc(m.name)}', '${esc(m.ip)}', ${m.mask})" title="Edit">\u270e</button>
                    <button onclick="event.stopPropagation(); deleteMachine('${esc(m.name)}')" title="Delete">\u2717</button>
                </span>
            </div>`;
        }).join('') : '<div class="empty-msg">no machines configured</div>';
        footer.innerHTML = '<button onclick="showMachineForm()">+ ADD</button><button onclick="showMachineImport()">IMPORT</button>';
    } else if (tab === 'flows') {
        let visible = flows;
        if (selectedMachine) {
            visible = flows.filter(f => f.source === selectedMachine || f.destination === selectedMachine);
        }
        if (flowSortBy) {
            visible = [...visible].sort((a, b) => a[flowSortBy].localeCompare(b[flowSortBy]));
        }
        const fAffected = getAnomalyAffectedMachines();
        content.innerHTML = visible.length ? visible.map(f => {
            const sel = (selectedFlow === f.name) ? ' selected' : '';
            const active = f.enabled ? ' active' : ' inactive';
            const anomaly = (fAffected.has(f.source) || fAffected.has(f.destination)) ? ' anomaly-affected' : '';
            const wave = f.enabled ? waveString() : '\u2500\u2500\u2500';
            const statusIcon = f.enabled ? '\u25a0' : '\u25b6';
            const toggleAction = f.enabled ? 'stop' : 'start';
            const toggleLabel = f.enabled ? 'STOP' : 'START';
            return `<div class="row flow-row${sel}${active}${anomaly}" data-name="${esc(f.name)}" onclick="selectFlow('${esc(f.name)}')">
                <span class="flow-endpoints">${esc(f.source)}:${f.source_port} \u2192 ${esc(f.destination)}:${f.destination_port}</span>
                <span class="flow-proto">${esc(f.protocol)}</span>
                <span class="flow-bar"><span class="flow-bar-fill" style="width:${flowBarWidth(f)}%"></span></span>
                <span class="flow-rate">${esc(f.rate)}</span>
                <span class="flow-wave" data-flow="${esc(f.name)}">${wave}</span>
                <span class="row-actions">
                    <button onclick="event.stopPropagation(); toggleFlow('${esc(f.name)}', '${toggleAction}')" title="${toggleLabel}">${statusIcon}</button>
                    <button onclick="event.stopPropagation(); showEditFlowForm('${esc(f.name)}')" title="Edit">\u270e</button>
                    <button onclick="event.stopPropagation(); deleteFlow('${esc(f.name)}')" title="Delete">\u2717</button>
                </span>
            </div>`;
        }).join('') : '<div class="empty-msg">no flows configured</div>';
        footer.innerHTML = `
            <button onclick="showFlowForm()">+ NEW FLOW</button>
            <button onclick="startAll()">START ALL</button>
            <button onclick="stopAll()">STOP ALL</button>
            <span class="sort-controls" style="margin-left:auto;">
                <button class="sort-btn${flowSortBy === 'source' ? ' active' : ''}" onclick="setFlowSort('source')">SRC</button>
                <button class="sort-btn${flowSortBy === 'destination' ? ' active' : ''}" onclick="setFlowSort('destination')">DST</button>
            </span>`;
    } else if (tab === 'map') {
        content.innerHTML = '<div class="map-viewport" id="fs-map-viewport" style="height:100%;"></div>';
        footer.innerHTML = '';
        // Move canvas into fullscreen map viewport
        const canvas = document.getElementById('network-map');
        const fsMapVp = document.getElementById('fs-map-viewport');
        if (canvas && fsMapVp) {
            fsMapVp.appendChild(canvas);
            // Trigger map resize
            if (typeof networkMap !== 'undefined' && networkMap.resize) {
                setTimeout(() => networkMap.resize(), 50);
            }
        }
    }
}

function restoreDashboard() {
    // Move canvas back to dashboard map viewport if needed
    const canvas = document.getElementById('network-map');
    const dashMapVp = document.getElementById('map-viewport');
    if (canvas && dashMapVp && !dashMapVp.contains(canvas)) {
        dashMapVp.appendChild(canvas);
        if (typeof networkMap !== 'undefined' && networkMap.resize) {
            setTimeout(() => networkMap.resize(), 50);
        }
    }
}

// ──── Status Bar ─────────────────────────────────────────────────────────────
function updateStatusBar() {
    const tpEl = document.getElementById('total-throughput');
    const afEl = document.getElementById('active-flows');
    const psEl = document.getElementById('packets-sec');
    const esEl = document.getElementById('engine-status');

    let activeCount = 0;
    let totalBps = 0;
    flows.forEach(f => {
        if (f.enabled) {
            activeCount++;
            totalBps += parseRateToBps(f.rate);
        }
    });

    // Estimate pkt/s: assume ~800 byte avg packet size
    const pktPerSec = totalBps > 0 ? Math.round(totalBps / 8 / 800) : 0;

    if (tpEl) tpEl.textContent = '\u2191' + formatBpsValue(totalBps);
    if (afEl) afEl.textContent = '\u25cf' + activeCount + ' flows';
    if (psEl) psEl.textContent = '\u2191' + formatCount(pktPerSec) + ' pkt/s';
    if (esEl) esEl.textContent = 'ENGINE: ' + (engineStatus.running ? 'RUNNING' : 'OFF');

    // Update globe animation state
    if (typeof setGlobeRunning === 'function') {
        setGlobeRunning(engineStatus.running);
    }
}

// ──── WebSocket Stats Handler ────────────────────────────────────────────────
function handleWSStats(msg) {
    if (msg.type === 'flow_stats') {
        const st = msg.data;
        // Update the matching flow's runtime stats
        if (st && st.FlowName) {
            // We could track per-flow stats; for now update the status bar
            updateRuntimeStats(st);
        }
    } else if (msg.type === 'exporter_stats') {
        if (msg.data) {
            updateExporterStats(msg.data);
        }
    } else if (msg.type === 'anomaly_started') {
        fetchActiveAnomalies();
    } else if (msg.type === 'anomaly_ended') {
        fetchActiveAnomalies();
    } else if (msg.type === 'anomaly_cleared') {
        activeAnomalies = [];
        renderAnomalyBanner();
    }
}

// Track runtime stats from WS — compute delta rates, not cumulative totals
let runtimeStats = { perFlow: {}, prevPerFlow: {}, lastUpdate: Date.now(), totalBps: 0, totalPps: 0 };

function updateRuntimeStats(st) {
    const now = Date.now();
    const name = st.FlowName;
    const curBytes = st.BytesSent || 0;
    const curPkts = st.PacketsSent || 0;

    // Store previous values for delta calculation
    const prev = runtimeStats.perFlow[name];
    runtimeStats.prevPerFlow[name] = prev ? { bytes: prev.bytes, packets: prev.packets, ts: prev.ts } : null;

    runtimeStats.perFlow[name] = {
        bytes: curBytes,
        packets: curPkts,
        active: st.Active || false,
        ts: now,
    };

    // Calculate aggregate rate from deltas across all flows
    let totalBps = 0;
    let totalPps = 0;
    for (const k in runtimeStats.perFlow) {
        const cur = runtimeStats.perFlow[k];
        const prv = runtimeStats.prevPerFlow[k];
        if (prv && cur.ts > prv.ts) {
            const dtSec = (cur.ts - prv.ts) / 1000;
            if (dtSec > 0 && dtSec < 120) { // ignore stale deltas
                const bytesDelta = cur.bytes - prv.bytes;
                const pktsDelta = cur.packets - prv.packets;
                if (bytesDelta >= 0 && pktsDelta >= 0) {
                    totalBps += (bytesDelta * 8) / dtSec;
                    totalPps += pktsDelta / dtSec;
                }
            }
        }
    }

    runtimeStats.totalBps = totalBps;
    runtimeStats.totalPps = totalPps;

    const tpEl = document.getElementById('total-throughput');
    const psEl = document.getElementById('packets-sec');
    if (tpEl) tpEl.textContent = '\u2191' + formatBpsValue(totalBps);
    if (psEl) psEl.textContent = '\u2191' + formatCount(Math.round(totalPps)) + ' pkt/s';
}

function updateExporterStats(data) {
    // Update collector stats inline
    for (const name in data) {
        const c = collectors.find(cc => cc.name === name);
        if (c) {
            c.packets_sent = data[name].PacketsSent || 0;
            c.bytes_sent = data[name].BytesSent || 0;
            c.errors = data[name].Errors || 0;
        }
    }
    renderCollectors();
}

// ──── Modal Form System ──────────────────────────────────────────────────────
function showModal(html) {
    const modal = document.getElementById('modal');
    const body = document.getElementById('modal-body');
    if (!modal || !body) return;
    body.innerHTML = html;
    modal.style.display = 'flex';
    // Focus first input
    const first = body.querySelector('input');
    if (first) setTimeout(() => first.focus(), 50);
}

function hideModal() {
    const modal = document.getElementById('modal');
    if (modal) modal.style.display = 'none';
}

// Close modal on backdrop click
document.addEventListener('click', (e) => {
    const modal = document.getElementById('modal');
    if (e.target === modal) hideModal();
});

// Close modal on Escape
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') hideModal();
});

// ── Machine Form ──
function showMachineForm() {
    showModal(`
        <div class="modal-title">ADD MACHINE</div>
        <label>NAME</label>
        <input type="text" id="m-name" placeholder="e.g. web-server-01">
        <label>IP ADDRESS</label>
        <input type="text" id="m-ip" placeholder="e.g. 10.0.1.10">
        <label>MASK (CIDR bits)</label>
        <input type="number" id="m-mask" value="24" min="0" max="32">
        <div class="modal-actions">
            <button onclick="submitMachine()">CREATE</button>
            <button onclick="hideModal()">CANCEL</button>
        </div>
    `);
}

async function submitMachine() {
    const name = document.getElementById('m-name').value.trim();
    const ip = document.getElementById('m-ip').value.trim();
    const mask = parseInt(document.getElementById('m-mask').value, 10) || 24;
    if (!name || !ip) return;
    await api('POST', '/api/machines', { name, ip, mask });
    hideModal();
    await fetchMachines();
}

// ── Edit Machine Form ──
function showEditMachineForm(oldName, oldIp, oldMask) {
    showModal(`
        <div class="modal-title">EDIT MACHINE</div>
        <input type="hidden" id="m-old-name" value="${esc(oldName)}">
        <label>NAME</label>
        <input type="text" id="m-name" value="${esc(oldName)}">
        <label>IP ADDRESS</label>
        <input type="text" id="m-ip" value="${esc(oldIp)}">
        <label>MASK (CIDR bits)</label>
        <input type="number" id="m-mask" value="${oldMask}" min="0" max="32">
        <div class="modal-actions">
            <button onclick="submitEditMachine()">SAVE</button>
            <button onclick="hideModal()">CANCEL</button>
        </div>
    `);
}

async function submitEditMachine() {
    const oldName = document.getElementById('m-old-name').value.trim();
    const name = document.getElementById('m-name').value.trim();
    const ip = document.getElementById('m-ip').value.trim();
    const mask = parseInt(document.getElementById('m-mask').value, 10) || 24;
    if (!name || !ip) return;
    await api('PUT', `/api/machines/${encodeURIComponent(oldName)}`, { name, ip, mask });
    hideModal();
    await fetchMachines();
}

// ── Machine Import ──
function showMachineImport() {
    showModal(`
        <div class="modal-title">IMPORT MACHINES</div>
        <label>CSV (no header — name, ip, mask)</label>
        <textarea id="m-import-csv" rows="10" style="width:100%; background:#000; border:1px solid #333; color:#8df776; font-family:'Share Tech Mono',monospace; font-size:12px; padding:6px; text-transform:none; resize:vertical;" placeholder="web-server,10.0.1.10,24
db-server,10.0.1.20,24
firewall,172.16.0.1,16"></textarea>
        <div class="modal-actions">
            <button onclick="submitMachineImport()">IMPORT</button>
            <button onclick="hideModal()">CANCEL</button>
        </div>
    `);
}

async function submitMachineImport() {
    const csv = document.getElementById('m-import-csv').value.trim();
    if (!csv) return;
    const lines = csv.split('\n').map(l => l.trim()).filter(l => l);
    let imported = 0;
    let errors = [];
    for (const line of lines) {
        const parts = line.split(',').map(s => s.trim());
        if (parts.length < 3) {
            errors.push(line + ' — need 3 fields');
            continue;
        }
        const [name, ip, maskStr] = parts;
        const mask = parseInt(maskStr, 10);
        if (!name || !ip || isNaN(mask)) {
            errors.push(line + ' — invalid fields');
            continue;
        }
        try {
            await api('POST', '/api/machines', { name, ip, mask });
            imported++;
        } catch (e) {
            errors.push(name + ' — ' + e.message);
        }
    }
    hideModal();
    await fetchMachines();
    if (errors.length) {
        alert('Imported ' + imported + ' machines.\n\nErrors:\n' + errors.join('\n'));
    }
}

// ── Flow Form ──
function machineOptions(selected) {
    const sorted = [...machines].sort((a, b) => a.name.localeCompare(b.name));
    return sorted.map(m => {
        const sel = (m.name === selected) ? ' selected' : '';
        return `<option value="${esc(m.name)}"${sel}>${esc(m.name)}</option>`;
    }).join('');
}

function showFlowForm() {
    showModal(`
        <div class="modal-title">NEW FLOW</div>
        <label>SOURCE</label>
        <select id="f-source">${machineOptions('')}</select>
        <label>SOURCE PORT</label>
        <input type="number" id="f-sport" value="443" min="0" max="65535">
        <label>DESTINATION</label>
        <select id="f-dest">${machineOptions('')}</select>
        <label>DESTINATION PORT</label>
        <input type="number" id="f-dport" value="5432" min="0" max="65535">
        <label>PROTOCOL</label>
        <select id="f-proto">
            <option value="TCP" selected>TCP</option>
            <option value="UDP">UDP</option>
        </select>
        <label>RATE</label>
        <input type="text" id="f-rate" value="1Mbps" placeholder="e.g. 1Mbps, 500Kbps">
        <label>CONNECTION STYLE</label>
        <select id="f-connstyle">
            <option value="persistent" selected>PERSISTENT (long-lived: SYN → ACK+PSH)</option>
            <option value="transactional">TRANSACTIONAL (short-lived: SYN+ACK+PSH+FIN)</option>
        </select>
        <label>FLUCTUATION AMPLITUDE (0.0-1.0)</label>
        <input type="text" id="f-fluct-amp" placeholder="0.3">
        <label>FLUCTUATION PERIOD (Go duration)</label>
        <input type="text" id="f-fluct-period" placeholder="1h">
        <label>FLUCTUATION PHASE (Go duration)</label>
        <input type="text" id="f-fluct-phase" placeholder="0s">
        <label>APP ID (optional)</label>
        <input type="number" id="f-appid" value="0" min="0">
        <div class="modal-actions">
            <button onclick="submitFlow()">CREATE</button>
            <button onclick="hideModal()">CANCEL</button>
        </div>
    `);
}

async function submitFlow() {
    const source = document.getElementById('f-source').value;
    const source_port = parseInt(document.getElementById('f-sport').value, 10) || 0;
    const destination = document.getElementById('f-dest').value;
    const destination_port = parseInt(document.getElementById('f-dport').value, 10) || 0;
    const protocol = document.getElementById('f-proto').value || 'TCP';
    const rate = document.getElementById('f-rate').value.trim() || '1Mbps';
    const connection_style = document.getElementById('f-connstyle').value || 'persistent';
    const app_id = parseInt(document.getElementById('f-appid').value, 10) || 0;

    const fluctAmp = document.getElementById('f-fluct-amp').value.trim();
    const fluctPeriod = document.getElementById('f-fluct-period').value.trim();
    const fluctPhase = document.getElementById('f-fluct-phase').value.trim();

    if (!source || !destination) return;
    let name = source + '-to-' + destination;
    if (flows.some(f => f.name === name)) {
        name = name + '-' + destination_port;
    }

    const body = {
        name, source, source_port, destination, destination_port,
        protocol, rate, connection_style, app_id, enabled: true,
    };
    if (fluctAmp) {
        body.fluctuation = {
            amplitude: parseFloat(fluctAmp) || 0,
            period: parseDurationToNs(fluctPeriod || '1h'),
            phase: parseDurationToNs(fluctPhase || '0s'),
        };
    }

    try {
        await api('POST', '/api/flows', body);
        hideModal();
        await fetchFlows();
    } catch (e) {
        alert('Failed to create flow: ' + e.message);
    }
}

// ── Edit Flow Form ──
function showEditFlowForm(name) {
    const f = flows.find(fl => fl.name === name);
    if (!f) return;
    const protoTCP = f.protocol === 'TCP' ? ' selected' : '';
    const protoUDP = f.protocol === 'UDP' ? ' selected' : '';
    const connPersistent = (f.connection_style || 'persistent') === 'persistent' ? ' selected' : '';
    const connTransactional = (f.connection_style || '') === 'transactional' ? ' selected' : '';
    showModal(`
        <div class="modal-title">EDIT FLOW</div>
        <input type="hidden" id="f-old-name" value="${esc(f.name)}">
        <label>SOURCE</label>
        <select id="f-source">${machineOptions(f.source)}</select>
        <label>SOURCE PORT</label>
        <input type="number" id="f-sport" value="${f.source_port}" min="0" max="65535">
        <label>DESTINATION</label>
        <select id="f-dest">${machineOptions(f.destination)}</select>
        <label>DESTINATION PORT</label>
        <input type="number" id="f-dport" value="${f.destination_port}" min="0" max="65535">
        <label>PROTOCOL</label>
        <select id="f-proto">
            <option value="TCP"${protoTCP}>TCP</option>
            <option value="UDP"${protoUDP}>UDP</option>
        </select>
        <label>RATE</label>
        <input type="text" id="f-rate" value="${esc(f.rate)}">
        <label>CONNECTION STYLE</label>
        <select id="f-connstyle">
            <option value="persistent"${connPersistent}>PERSISTENT (long-lived: SYN \u2192 ACK+PSH)</option>
            <option value="transactional"${connTransactional}>TRANSACTIONAL (short-lived: SYN+ACK+PSH+FIN)</option>
        </select>
        <label>FLUCTUATION AMPLITUDE (0.0-1.0)</label>
        <input type="text" id="f-fluct-amp" placeholder="0.3">
        <label>FLUCTUATION PERIOD (Go duration)</label>
        <input type="text" id="f-fluct-period" placeholder="1h">
        <label>FLUCTUATION PHASE (Go duration)</label>
        <input type="text" id="f-fluct-phase" placeholder="0s">
        <label>APP ID (optional)</label>
        <input type="number" id="f-appid" value="${f.app_id || 0}" min="0">
        <div class="modal-actions">
            <button onclick="submitEditFlow()">SAVE</button>
            <button onclick="hideModal()">CANCEL</button>
        </div>
    `);
}

async function submitEditFlow() {
    const oldName = document.getElementById('f-old-name').value.trim();
    const source = document.getElementById('f-source').value;
    const source_port = parseInt(document.getElementById('f-sport').value, 10) || 0;
    const destination = document.getElementById('f-dest').value;
    const destination_port = parseInt(document.getElementById('f-dport').value, 10) || 0;
    const protocol = document.getElementById('f-proto').value || 'TCP';
    const rate = document.getElementById('f-rate').value.trim() || '1Mbps';
    const connection_style = document.getElementById('f-connstyle').value || 'persistent';
    const app_id = parseInt(document.getElementById('f-appid').value, 10) || 0;

    const fluctAmp = document.getElementById('f-fluct-amp').value.trim();
    const fluctPeriod = document.getElementById('f-fluct-period').value.trim();
    const fluctPhase = document.getElementById('f-fluct-phase').value.trim();

    if (!source || !destination) return;
    const name = source + '-to-' + destination;

    const body = {
        name, source, source_port, destination, destination_port,
        protocol, rate, connection_style, app_id, enabled: true,
    };
    if (fluctAmp) {
        body.fluctuation = {
            amplitude: parseFloat(fluctAmp) || 0,
            period: parseDurationToNs(fluctPeriod || '1h'),
            phase: parseDurationToNs(fluctPhase || '0s'),
        };
    }

    try {
        await api('PUT', `/api/flows/${encodeURIComponent(oldName)}`, body);
        hideModal();
        await fetchFlows();
    } catch (e) {
        alert('Failed to update flow: ' + e.message);
    }
}

// ── Collector Form ──
function showCollectorForm() {
    showModal(`
        <div class="modal-title">ADD COLLECTOR</div>
        <label>NAME</label>
        <input type="text" id="c-name" placeholder="e.g. collector-01">
        <label>ADDRESS (host:port)</label>
        <input type="text" id="c-address" placeholder="e.g. 10.0.1.50:2055">
        <label>VERSION</label>
        <select id="c-version">
            <option value="v9" selected>NetFlow v9</option>
            <option value="v10">IPFIX (v10)</option>
        </select>
        <div class="modal-actions">
            <button onclick="submitCollector()">CREATE</button>
            <button onclick="hideModal()">CANCEL</button>
        </div>
    `);
}

async function submitCollector() {
    const name = document.getElementById('c-name').value.trim();
    const address = document.getElementById('c-address').value.trim();
    const version = document.getElementById('c-version').value || 'v9';
    if (!name || !address) return;
    await api('POST', '/api/collectors', { name, address, version });
    hideModal();
    await fetchCollectors();
}

// ── Edit Collector Form ──
function showEditCollectorForm(name) {
    const c = collectors.find(cc => cc.name === name);
    if (!c) return;
    const v9Sel = c.version === 'v9' ? 'selected' : '';
    const v10Sel = c.version === 'v10' ? 'selected' : '';
    showModal(`
        <div class="modal-title">EDIT COLLECTOR</div>
        <input type="hidden" id="c-old-name" value="${esc(c.name)}">
        <label>NAME</label>
        <input type="text" id="c-name" value="${esc(c.name)}">
        <label>ADDRESS (host:port)</label>
        <input type="text" id="c-address" value="${esc(c.address)}">
        <label>VERSION</label>
        <select id="c-version">
            <option value="v9" ${v9Sel}>NetFlow v9</option>
            <option value="v10" ${v10Sel}>IPFIX (v10)</option>
        </select>
        <div class="modal-actions">
            <button onclick="submitEditCollector()">SAVE</button>
            <button onclick="hideModal()">CANCEL</button>
        </div>
    `);
}

async function submitEditCollector() {
    const oldName = document.getElementById('c-old-name').value.trim();
    const name = document.getElementById('c-name').value.trim();
    const address = document.getElementById('c-address').value.trim();
    const version = document.getElementById('c-version').value || 'v9';
    if (!name || !address) return;
    // Delete old and create new (no PUT endpoint for collectors)
    await api('DELETE', `/api/collectors/${encodeURIComponent(oldName)}`);
    await api('POST', '/api/collectors', { name, address, version });
    hideModal();
    await fetchCollectors();
}

// ──── Anomaly System ────────────────────────────────────────────────────────
async function fetchAnomalyScenarios() {
    try { anomalyScenarios = await api('GET', '/api/anomaly/scenarios') || []; } catch (e) { anomalyScenarios = []; }
    renderAnomalyScenarios();
}

async function fetchActiveAnomalies() {
    try { activeAnomalies = await api('GET', '/api/anomaly/active') || []; } catch (e) { activeAnomalies = []; }
    renderAnomalyBanner();
    renderMachines();
    renderFlows();
}

function renderAnomalyScenarios() {
    const el = document.getElementById('anomaly-scenarios');
    if (!el || !anomalyScenarios.length) return;

    el.innerHTML = anomalyScenarios.map(s => {
        return `<div class="anomaly-scenario" onclick="showAnomalyTweakForm('${esc(s.type)}')">
            <div class="scenario-category"></div>
            <div class="scenario-info">
                <div class="scenario-name">${esc(s.name)}</div>
                <div class="scenario-desc">${esc(s.description)}</div>
            </div>
            <div class="scenario-defaults">${esc(s.default_duration)}<br>${s.default_intensity}x</div>
            <button class="scenario-fire" onclick="event.stopPropagation(); fireAnomaly('${esc(s.type)}')">FIRE</button>
        </div>`;
    }).join('');
}

function scenarioCategory(type) {
    const attack = ['ddos', 'port_scan', 'lateral_movement'];
    const volume = ['data_exfiltration', 'bandwidth_spike', 'traffic_blackout'];
    if (attack.includes(type)) return 'attack';
    if (volume.includes(type)) return 'volume';
    return 'pattern';
}

function showAnomalyTweakForm(type) {
    const s = anomalyScenarios.find(sc => sc.type === type);
    if (!s) return;
    const cat = scenarioCategory(type);

    const machineOpts = machines.map(m => `<option value="${esc(m.name)}">${esc(m.name)}</option>`).join('');

    showModal(`
        <div class="modal-title">${esc(s.name)}</div>
        <div style="color:#888; font-size:11px; margin-bottom:12px;">${esc(s.description)}</div>
        <input type="hidden" id="a-type" value="${esc(s.type)}">
        <label>DURATION</label>
        <input type="text" id="a-duration" value="${esc(s.default_duration)}">
        <label>INTENSITY (multiplier)</label>
        <input type="text" id="a-intensity" value="${s.default_intensity}">
        <label>TARGETS (optional — select machines)</label>
        <select id="a-targets" multiple size="4">
            <option value="">ALL MACHINES</option>
            ${machineOpts}
        </select>
        <label>COUNT (synthetic flows/ports)</label>
        <input type="number" id="a-count" value="${s.default_count}" min="0">
        <div class="modal-actions">
            <button onclick="submitAnomaly()">FIRE</button>
            <button onclick="hideModal()">CANCEL</button>
        </div>
    `);
}

async function submitAnomaly() {
    const type = document.getElementById('a-type').value;
    const duration = document.getElementById('a-duration').value.trim();
    const intensity = parseFloat(document.getElementById('a-intensity').value) || 0;
    const countVal = parseInt(document.getElementById('a-count').value, 10) || 0;

    const targetsEl = document.getElementById('a-targets');
    const targets = [];
    if (targetsEl) {
        for (const opt of targetsEl.selectedOptions) {
            if (opt.value) targets.push(opt.value);
        }
    }

    try {
        await api('POST', '/api/anomaly/start', {
            scenario: type, duration, intensity, targets, count: countVal
        });
        hideModal();
        await fetchActiveAnomalies();
    } catch (e) {
        alert('Failed to start anomaly: ' + e.message);
    }
}

async function fireAnomaly(type) {
    const s = anomalyScenarios.find(sc => sc.type === type);
    if (!s) return;
    try {
        await api('POST', '/api/anomaly/start', {
            scenario: type,
            duration: s.default_duration,
            intensity: s.default_intensity,
            count: s.default_count,
            targets: []
        });
        await fetchActiveAnomalies();
    } catch (e) {
        alert('Failed: ' + e.message);
    }
}

async function clearAllAnomalies() {
    try {
        await api('POST', '/api/anomaly/clear');
        activeAnomalies = [];
        renderAnomalyBanner();
    } catch (e) { /* ignore */ }
}

async function stopAnomaly(id) {
    try {
        await api('POST', '/api/anomaly/stop', { id });
        await fetchActiveAnomalies();
    } catch (e) { /* ignore */ }
}

function renderAnomalyBanner() {
    const banner = document.getElementById('anomaly-banner');
    const btn = document.getElementById('anomaly-btn');
    const clearBtn = document.getElementById('clear-anomalies-btn');

    if (!banner) return;

    if (!activeAnomalies || activeAnomalies.length === 0) {
        banner.style.display = 'none';
        if (btn) btn.classList.remove('has-active');
        if (clearBtn) clearBtn.style.display = 'none';
        return;
    }

    banner.style.display = 'flex';
    if (btn) btn.classList.add('has-active');
    if (clearBtn) clearBtn.style.display = '';

    const tags = activeAnomalies.map(a => {
        return `<span class="anomaly-tag">${esc(a.name)} (${esc(a.remaining)})
            <button onclick="stopAnomaly('${esc(a.id)}')" style="background:none;border:none;color:inherit;cursor:pointer;font-size:12px;padding:0 0 0 4px;">\u2717</button>
        </span>`;
    }).join('');

    banner.innerHTML = `<span>ANOMALY:</span> ${tags}
        <button class="clear-all-btn" onclick="clearAllAnomalies()">CLEAR ALL</button>`;
}

function toggleAnomalyDrawer() {
    const drawer = document.getElementById('anomaly-drawer');
    const btn = document.getElementById('anomaly-btn');
    if (!drawer) return;
    // Close other drawers
    const fileDrawer = document.getElementById('file-drawer');
    if (fileDrawer && fileDrawer.classList.contains('open')) {
        fileDrawer.classList.remove('open');
        document.getElementById('file-btn')?.classList.remove('active');
    }
    const collDrawer = document.getElementById('collector-drawer');
    if (collDrawer && collDrawer.classList.contains('open')) {
        collDrawer.classList.remove('open');
        document.getElementById('collector-btn')?.classList.remove('active');
    }
    drawer.classList.toggle('open');
    if (btn) btn.classList.toggle('active', drawer.classList.contains('open'));
    if (drawer.classList.contains('open')) {
        fetchAnomalyScenarios();
        fetchActiveAnomalies();
    }
}

// Poll active anomalies to update countdown timers
let anomalyPollInterval = null;
function startAnomalyPolling() {
    if (anomalyPollInterval) clearInterval(anomalyPollInterval);
    anomalyPollInterval = setInterval(() => {
        if (activeAnomalies.length > 0) fetchActiveAnomalies();
    }, 2000);
}

// ──── Utility Functions ──────────────────────────────────────────────────────
function esc(s) {
    if (!s) return '';
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function parseDurationToNs(s) {
    if (!s) return 0;
    let total = 0;
    const re = /(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g;
    let match;
    while ((match = re.exec(s)) !== null) {
        const val = parseFloat(match[1]);
        const unit = match[2];
        switch (unit) {
            case 'ns': total += val; break;
            case 'us': case 'µs': total += val * 1e3; break;
            case 'ms': total += val * 1e6; break;
            case 's': total += val * 1e9; break;
            case 'm': total += val * 60e9; break;
            case 'h': total += val * 3600e9; break;
        }
    }
    return total;
}

function parseRateToBps(rate) {
    const num = parseFloat(rate) || 0;
    const unit = (rate || '').replace(/[\d.]/g, '').toLowerCase();
    if (unit.startsWith('g')) return num * 1e9;
    if (unit.startsWith('m')) return num * 1e6;
    if (unit.startsWith('k')) return num * 1e3;
    return num;
}

function formatBpsValue(bps) {
    if (bps >= 1e9) return (bps / 1e9).toFixed(1) + 'Gbps';
    if (bps >= 1e6) return (bps / 1e6).toFixed(1) + 'Mbps';
    if (bps >= 1e3) return (bps / 1e3).toFixed(1) + 'Kbps';
    return Math.round(bps) + 'bps';
}

function formatBps(bytes) {
    const bits = bytes * 8;
    if (bits >= 1e9) return (bits / 1e9).toFixed(1) + 'Gbps';
    if (bits >= 1e6) return (bits / 1e6).toFixed(1) + 'Mbps';
    if (bits >= 1e3) return (bits / 1e3).toFixed(1) + 'Kbps';
    return bits + 'bps';
}

function formatCount(n) {
    if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
    return String(n);
}

// ──── Waveform Animation ─────────────────────────────────────────────────────
function animateWaveforms() {
    waveFrame++;
    const waveEls = document.querySelectorAll('.flow-wave');
    waveEls.forEach(el => {
        const flowName = el.getAttribute('data-flow');
        const flow = flows.find(f => f.name === flowName);
        if (flow && flow.enabled) {
            el.textContent = waveString();
        }
    });
}

let waveInterval = null;
function startWaveAnimation() {
    if (waveInterval) clearInterval(waveInterval);
    waveInterval = setInterval(animateWaveforms, 200);
}

// ──── Initialization ─────────────────────────────────────────────────────────
async function init() {
    // Connect WebSocket
    flowboyWS.onStats = handleWSStats;
    flowboyWS.connect();

    // Initialize wireframe globe
    initGlobe('globe');

    // Initial data fetch
    await Promise.all([
        fetchMachines(),
        fetchFlows(),
        fetchCollectors(),
        fetchEngineStatus(),
        fetchConfigs(),
        fetchAnomalyScenarios(),
        fetchActiveAnomalies(),
        fetchFluctuation(),
    ]);

    // Highlight default sort button
    document.querySelectorAll('.sort-btn').forEach(btn => {
        if (btn.textContent === 'SRC') btn.classList.add('active');
    });

    // Start waveform animation
    startWaveAnimation();

    // Start anomaly polling for countdown updates
    startAnomalyPolling();

    // Periodic engine status refresh
    setInterval(fetchEngineStatus, 5000);

    // Resize handle drag
    initResizeHandle();
}

// ──── Resize Handle ─────────────────────────────────────────────────────────
function initResizeHandle() {
    const handle = document.getElementById('resize-handle');
    const topRow = document.querySelector('.top-row');
    const bottomRow = document.getElementById('bottom-row');
    const dashboard = document.querySelector('.dashboard');
    if (!handle || !topRow || !bottomRow || !dashboard) return;

    let dragging = false;
    let startY = 0;
    let startTopFlex = 0;
    let startBottomFlex = 0;

    handle.addEventListener('mousedown', (e) => {
        dragging = true;
        startY = e.clientY;
        const dashRect = dashboard.getBoundingClientRect();
        const totalH = dashRect.height;
        startTopFlex = topRow.getBoundingClientRect().height / totalH;
        startBottomFlex = bottomRow.getBoundingClientRect().height / totalH;
        handle.classList.add('dragging');
        e.preventDefault();
    });

    document.addEventListener('mousemove', (e) => {
        if (!dragging) return;
        const dashRect = dashboard.getBoundingClientRect();
        const totalH = dashRect.height;
        const delta = (e.clientY - startY) / totalH;
        const newTop = Math.max(0.15, Math.min(0.85, startTopFlex + delta));
        const newBottom = Math.max(0.05, Math.min(0.75, startBottomFlex - delta));
        topRow.style.flex = `${newTop * 10} 1 0px`;
        bottomRow.style.flex = `${newBottom * 10} 1 0px`;
    });

    document.addEventListener('mouseup', () => {
        if (!dragging) return;
        dragging = false;
        handle.classList.remove('dragging');
    });
}

// Start when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
