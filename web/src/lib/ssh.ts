// Client side of the /ws protocol documented in core/session.go.

export interface ConnectOptions {
	host: string;
	port: number;
	user: string;
	password?: string;
	keyId?: string; // a key saved in the vault (preferred)
	privateKey?: string; // an unsaved, pasted key
	passphrase?: string;
	cols: number;
	rows: number;
}

export interface HostKeyPrompt {
	status: 'new' | 'changed';
	keyType: string;
	fingerprint: string;
}

export type ConnState = 'connecting' | 'verify' | 'ready' | 'closed' | 'error';

export interface Handlers {
	onData(bytes: Uint8Array): void;
	onState(state: ConnState, detail?: string): void;
	onHostKey(prompt: HostKeyPrompt): void;
}

export class SSHConnection {
	private ws: WebSocket;
	private encoder = new TextEncoder();
	private finished = false; // an error/exit message already explained the end

	constructor(opts: ConnectOptions, private h: Handlers, wsUrl = defaultWsUrl()) {
		this.ws = new WebSocket(wsUrl);
		this.ws.binaryType = 'arraybuffer';
		this.ws.onopen = () => this.json({ type: 'connect', ...opts });
		this.ws.onmessage = (ev) => {
			if (ev.data instanceof ArrayBuffer) return this.h.onData(new Uint8Array(ev.data));
			const m = JSON.parse(ev.data as string);
			switch (m.type) {
				case 'hostkey':
					this.h.onState('verify');
					this.h.onHostKey(m);
					break;
				case 'ready':
					this.h.onState('ready');
					break;
				case 'error':
					this.finished = true;
					this.h.onState('error', m.message);
					break;
				case 'exit':
					this.finished = true;
					this.h.onState('closed', m.message);
					break;
			}
		};
		this.ws.onclose = (ev) => {
			if (this.finished) return;
			// Normally an error/exit message arrives first and explains why; this is the fallback.
			const why = ev.reason ? `: ${ev.reason}` : '';
			const hint = ev.code === 1006 ? ' (socket dropped; is the Nasc core still running?)' : '';
			this.h.onState('closed', `connection lost [code ${ev.code}]${why}${hint}`);
		};
		this.h.onState('connecting');
	}

	private json(m: object) {
		if (this.ws.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(m));
	}

	trust(accept: boolean) {
		this.json({ type: 'trust', accept });
	}

	write(data: string | Uint8Array) {
		if (this.ws.readyState !== WebSocket.OPEN) return;
		this.ws.send(typeof data === 'string' ? this.encoder.encode(data) : (data as Uint8Array<ArrayBuffer>));
	}

	resize(cols: number, rows: number) {
		this.json({ type: 'resize', cols, rows });
	}

	close() {
		this.ws.onclose = null;
		this.ws.close();
	}
}

function defaultWsUrl() {
	return `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`;
}

/** Turn a typed character into its control code (Ctrl+C -> \x03); null if it has none. */
export function ctrlChar(ch: string): string | null {
	if (ch.length !== 1) return null;
	const code = ch.toUpperCase().charCodeAt(0);
	if (code >= 64 && code <= 95) return String.fromCharCode(code - 64);
	return null;
}
