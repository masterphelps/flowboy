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

const GLOBE_R = 9;            // sphere radius in px
const GLOBE_CX = 12;         // canvas center X
const GLOBE_CY = 12;         // canvas center Y
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
