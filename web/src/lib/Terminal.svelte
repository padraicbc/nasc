<script lang="ts">
	import { onMount } from 'svelte';
	import { Terminal } from '@xterm/xterm';
	import { FitAddon } from '@xterm/addon-fit';
	import { WebLinksAddon } from '@xterm/addon-web-links';
	import '@xterm/xterm/css/xterm.css';
	import { SSHConnection, ctrlChar, type ConnState, type HostKeyPrompt } from './ssh';
	import { tabs, type Tab } from './tabs.svelte';
	import { addHistory, getHistory, historyKey } from './api';
	import { LineTracker, OutputTail, anchorAt, completion, readCommand, remember, suggest, type Anchor } from './autocomplete';

	let { tab, active, onexit }: { tab: Tab; active: boolean; onexit: () => void } = $props();

	let el: HTMLDivElement;
	let term: Terminal;
	let fit: FitAddon;
	let conn: SSHConnection | null = null;
	let status = $state<ConnState>('connecting');
	let detail = $state('');
	let prompt = $state<HostKeyPrompt | null>(null);
	let ctrl = $state(false);

	// Autocomplete state (see autocomplete.ts).
	const tracker = new LineTracker();
	const tail = new OutputTail();
	// svelte-ignore state_referenced_locally
	const key = historyKey(tab.opts);
	let history = $state<string[]>([]);
	let line = $state(''); // what is on the command line right now, read from the screen
	let altScreen = $state(false); // full-screen apps (vim, htop): typing isn't a shell command
	let anchor: Anchor | null = null; // where the prompt ended
	let typing = false; // true from the first keystroke of a line until Enter/Ctrl+C (anchor is frozen)
	const suggestions = $derived(!altScreen ? suggest(history, line) : []);

	const keys: [string, string][] = [
		['Esc', '\x1b'],
		['Tab', '\t'],
		['↑', '\x1b[A'],
		['↓', '\x1b[B'],
		['←', '\x1b[D'],
		['→', '\x1b[C'],
		['|', '|'],
		['~', '~'],
		['/', '/'],
		['-', '-']
	];

	function connect() {
		conn?.close();
		prompt = null;
		detail = '';
		term.reset();
		fit.fit();
		conn = new SSHConnection(
			{ ...tab.opts, cols: term.cols, rows: term.rows },
			{
				onData: (b) => {
					tail.push(b);
					term.write(b);
				},
				onState: (s, d) => {
					status = s;
					detail = d ?? '';
					tabs.setStatus(tab.id, s, d ?? '');
					if (s === 'ready') {
						term.focus();
						getHistory(key).then((h) => (history = h)).catch(() => {});
					}
				},
				onHostKey: (p) => (prompt = p)
			}
		);
	}

	/** Re-read the command line from the screen (cheap; called after output is parsed). */
	function refresh() {
		altScreen = term.buffer.active.type === 'alternate';
		// While the user hasn't typed, the cursor sits at the end of the prompt: keep following it.
		if (!typing && !altScreen) anchor = anchorAt(term.buffer.active);
		const cmd = anchor && !altScreen ? readCommand(term.buffer.active, anchor) : null;
		line = cmd ?? '';
	}

	/** Tab accepts the top suggestion when one is showing; otherwise it goes to the remote shell. */
	function accept(suggestion = suggestions[0]) {
		if (!suggestion) return false;
		conn?.write(completion(line, suggestion)); // the echo updates the screen, which updates `line`
		term.focus();
		return true;
	}

	function send(data: string) {
		if (ctrl) {
			const c = ctrlChar(data);
			if (c) data = c;
			ctrl = false;
		}
		if (data === '\t' && accept()) return;
		if (!altScreen) {
			anchor ??= anchorAt(term.buffer.active);
			typing = true; // freeze the anchor so our own echo doesn't move it
			// Seed the tracker with what is on screen, then apply this chunk (a paste may contain Enter).
			tracker.set(readCommand(term.buffer.active, anchor) ?? '');
			const secret = tail.atSecretPrompt();
			for (const cmd of tracker.feed(data)) {
				if (secret) continue; // never store what was typed at a password prompt
				history = remember(history, cmd);
				addHistory(key, cmd).catch(() => {});
			}
			if (/[\r\n\x03\x04]/.test(data)) {
				anchor = null; // a new prompt comes next; refresh() re-anchors when it is drawn
				typing = false;
				line = '';
			}
		}
		conn?.write(data);
	}

	function sendKey(seq: string) {
		send(seq);
		term.focus();
	}

	onMount(() => {
		term = new Terminal({
			cursorBlink: true,
			fontSize: 14,
			scrollback: 5000,
			fontFamily: 'ui-monospace, Menlo, Consolas, monospace',
			theme: { background: '#0d1117', foreground: '#e6edf3' }
		});
		fit = new FitAddon();
		term.loadAddon(fit);
		term.loadAddon(new WebLinksAddon());
		term.open(el);
		term.onData(send);
		term.onWriteParsed(refresh);
		term.onResize(({ cols, rows }) => conn?.resize(cols, rows));

		const ro = new ResizeObserver(() => active && fit.fit());
		ro.observe(el);
		connect();
		return () => {
			ro.disconnect();
			conn?.close();
			term.dispose();
		};
	});

	$effect(() => {
		if (active && term) {
			fit.fit();
			term.focus();
		}
	});

	const answer = (accept: boolean) => {
		conn?.trust(accept);
		prompt = null;
	};
</script>

<div class="wrap" class:hidden={!active}>
	<div class="term" bind:this={el}></div>

	{#if suggestions.length > 0}
		<div class="suggest" aria-label="Suggestions from history">
			{#each suggestions as sug, i (sug)}
				<button
					class:first={i === 0}
					onmousedown={(e) => e.preventDefault()}
					onclick={() => accept(sug)}
				>
					<b>{line}</b>{completion(line, sug)}
				</button>
			{/each}
		</div>
	{/if}

	<div class="keys">
		<button class:on={ctrl} onmousedown={(e) => e.preventDefault()} onclick={() => (ctrl = !ctrl)}>Ctrl</button>
		{#each keys as [label, seq]}
			<button onmousedown={(e) => e.preventDefault()} onclick={() => sendKey(seq)}>{label}</button>
		{/each}
	</div>

	{#if prompt}
		<div class="overlay">
			<div class="card">
				<h3>{prompt.status === 'changed' ? '⚠ Host key changed' : 'New host'}</h3>
				{#if prompt.status === 'changed'}
					<p class="warn">
						The key for this server is different from the one you trusted before. This could be a
						man-in-the-middle attack, or the server was reinstalled.
					</p>
				{:else}
					<p>First connection to {tab.opts.host}. Verify the fingerprint.</p>
				{/if}
				<p class="fp">{prompt.keyType}<br />{prompt.fingerprint}</p>
				<div class="row">
					<button onclick={() => answer(false)}>Cancel</button>
					<button class:danger={prompt.status === 'changed'} class="primary" onclick={() => answer(true)}>
						{prompt.status === 'changed' ? 'Trust new key' : 'Trust & connect'}
					</button>
				</div>
			</div>
		</div>
	{:else if status === 'connecting'}
		<div class="overlay"><p class="muted">Connecting to {tab.opts.host}…</p></div>
	{:else if status === 'error' || status === 'closed'}
		<div class="overlay">
			<div class="card">
				<h3>{status === 'error' ? 'Connection failed' : 'Disconnected'}</h3>
				<p class:warn={status === 'error'}>{detail}</p>
				<div class="row">
					<button onclick={onexit}>Close tab</button>
					<button class="primary" onclick={connect}>Reconnect</button>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.wrap { position: relative; display: flex; flex-direction: column; flex: 1; min-height: 0; }
	.hidden { display: none; }
	.term { flex: 1; min-height: 0; padding: 4px; }
	.suggest { display: flex; gap: 4px; overflow-x: auto; padding: 4px; background: var(--panel); border-top: 1px solid var(--line); }
	.suggest button { flex: 0 0 auto; max-width: 80%; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-height: 36px; padding: 0.2rem 0.7rem; font: 0.85rem ui-monospace, monospace; color: var(--muted); }
	.suggest button b { color: var(--text); font-weight: 600; }
	.suggest button.first { border-color: var(--accent); }
	.keys { display: flex; gap: 4px; overflow-x: auto; padding: 4px; background: var(--panel); border-top: 1px solid var(--line); }
	.keys button { min-width: 44px; padding: 0.4rem 0.6rem; flex: 0 0 auto; }
	.keys button.on { background: var(--accent); border-color: var(--accent); }
	.overlay { position: absolute; inset: 0; display: grid; place-items: center; background: #0d1117cc; padding: 16px; }
	.card { background: var(--panel); border: 1px solid var(--line); border-radius: 12px; padding: 1rem; max-width: 420px; width: 100%; }
	.card h3 { margin: 0 0 0.5rem; }
	.row { display: flex; gap: 8px; justify-content: flex-end; margin-top: 1rem; }
	.fp { font: 0.8rem ui-monospace, monospace; word-break: break-all; color: var(--muted); }
	.warn { color: var(--danger); }
	.muted { color: var(--muted); }
</style>
