import type { ConnectOptions, ConnState } from './ssh';

export interface Tab {
	id: number;
	title: string;
	opts: ConnectOptions;
	status: ConnState;
	detail: string; // why it ended, if it did
	since: number | null; // when it became ready (ms since epoch)
}

let nextId = 1;

class Tabs {
	list = $state<Tab[]>([]);
	active = $state<number | null>(null);

	open(title: string, opts: Omit<ConnectOptions, 'cols' | 'rows'>) {
		const tab: Tab = {
			id: nextId++,
			title,
			opts: { ...opts, cols: 80, rows: 24 },
			status: 'connecting',
			detail: '',
			since: null
		};
		this.list.push(tab);
		this.active = tab.id;
	}

	/** Called by each terminal as its connection changes state. */
	setStatus(id: number, status: ConnState, detail = '') {
		const t = this.list.find((t) => t.id === id);
		if (!t) return;
		t.status = status;
		t.detail = detail;
		if (status === 'ready') t.since = Date.now();
		else if (status === 'connecting') t.since = null;
	}

	close(id: number) {
		const i = this.list.findIndex((t) => t.id === id);
		if (i < 0) return;
		this.list.splice(i, 1);
		if (this.active === id) this.active = this.list[Math.min(i, this.list.length - 1)]?.id ?? null;
	}
}

export const tabs = new Tabs();
