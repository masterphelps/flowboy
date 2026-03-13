// Flowboy 3000 - WebSocket Client
class FlowboyWS {
    constructor() {
        this.ws = null;
        this.onStats = null;
        this.reconnectDelay = 1000;
        this._closed = false;
    }

    connect() {
        this._closed = false;
        const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        this.ws = new WebSocket(`${proto}//${window.location.host}/ws`);

        this.ws.onopen = () => {
            this.reconnectDelay = 1000;
        };

        this.ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                if (this.onStats) this.onStats(data);
            } catch (e) {
                console.error('ws: bad message', e);
            }
        };

        this.ws.onclose = () => {
            if (this._closed) return;
            setTimeout(() => this.connect(), this.reconnectDelay);
            this.reconnectDelay = Math.min(this.reconnectDelay * 1.5, 10000);
        };

        this.ws.onerror = () => {
            this.ws.close();
        };
    }

    close() {
        this._closed = true;
        if (this.ws) this.ws.close();
    }
}

const flowboyWS = new FlowboyWS();
