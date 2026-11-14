export interface Host {
	id: string;
	name: string;
	host: string;
	port: number;
	user: string;
	group?: string;
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
	const res = await fetch(path, {
		method,
		headers: body ? { 'Content-Type': 'application/json' } : undefined,
		body: body ? JSON.stringify(body) : undefined
	});
	if (res.status === 204) return undefined as T;
	const data = await res.json().catch(() => ({}));
	if (!res.ok) throw new Error(data.error ?? `request failed (${res.status})`);
	return data as T;
}

export const listHosts = () => call<Host[]>('GET', '/api/hosts');
export const saveHost = (h: Partial<Host>) =>
	h.id ? call<Host>('PUT', `/api/hosts/${h.id}`, h) : call<Host>('POST', '/api/hosts', h);
export const deleteHost = (id: string) => call<void>('DELETE', `/api/hosts/${id}`);

export const historyKey = (o: { user: string; host: string; port: number }) => `${o.user}@${o.host}:${o.port}`;
export const getHistory = (key: string) => call<string[]>('GET', `/api/history?key=${encodeURIComponent(key)}`);
export const addHistory = (key: string, cmd: string) => call<void>('POST', '/api/history', { key, cmd });
export const clearHistory = (key: string) => call<void>('DELETE', `/api/history?key=${encodeURIComponent(key)}`);

export interface KeyInfo {
	id: string;
	name: string;
	type?: string;
	fingerprint?: string;
	publicKey?: string; // authorized_keys line
	encrypted: boolean;
}
export const listKeys = () => call<KeyInfo[]>('GET', '/api/keys');
export const addKey = (name: string, privateKey: string) => call<KeyInfo>('POST', '/api/keys', { name, privateKey });
export const deleteKey = (id: string) => call<void>('DELETE', `/api/keys/${id}`);
export const generateKey = (name: string, type: 'ed25519' | 'rsa', passphrase: string) =>
	call<KeyInfo>('POST', '/api/keys/generate', { name, type, passphrase });
