<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { deleteHost, listHosts, saveHost, type Host } from '../lib/api';
	import { tabs } from '../lib/tabs.svelte';
	import Terminal from '../lib/Terminal.svelte';
	import HostForm from '../lib/HostForm.svelte';
	import ConnectDialog from '../lib/ConnectDialog.svelte';
	import KeysView from '../lib/KeysView.svelte';
	import SettingsView from '../lib/SettingsView.svelte';
	import { pushBack } from '../lib/nav';

	let hosts = $state<Host[]>([]);
	let error = $state('');
	let editing = $state<Partial<Host> | null>(null);
	let connecting = $state<Host | null>(null);
	let showKeys = $state(false);
	let showSettings = $state(false);

	const refresh = async () => {
		try {
			hosts = await listHosts();
		} catch (e) {
			error = (e as Error).message;
		}
	};
	onMount(refresh);

	async function save(h: Partial<Host>) {
		try {
			await saveHost(h);
			editing = null;
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function remove() {
		if (!editing?.id || !confirm(`Delete ${editing.name}?`)) return;
		await deleteHost(editing.id);
		editing = null;
		await refresh();
	}

	function connect(secrets: { password?: string; keyId?: string; passphrase?: string }) {
		const h = connecting!;
		tabs.open(h.name, { host: h.host, port: h.port, user: h.user, ...secrets });
		connecting = null;
	}

	const showingList = $derived(tabs.active === null);

	// Android back: close the current sub-screen / leave the terminal for the host list.
	$effect(() => { if (editing) return pushBack(() => (editing = null)); });
	$effect(() => { if (connecting) return pushBack(() => (connecting = null)); });
	$effect(() => { if (showKeys) return pushBack(() => (showKeys = false)); });
	$effect(() => { if (showSettings) return pushBack(() => (showSettings = false)); });
	$effect(() => { if (tabs.active !== null) return pushBack(() => (tabs.active = null)); });

	// Re-render "connected 5m" labels every so often.
	let now = $state(Date.now());
	onMount(() => {
		const t = setInterval(() => (now = Date.now()), 15000);
		return () => clearInterval(t);
	});

	const label: Record<string, string> = {
		connecting: 'Connecting…',
		verify: 'Verify host key',
		ready: 'Connected',
		closed: 'Disconnected',
		error: 'Failed'
	};

	function age(since: number | null) {
		if (!since) return '';
		const s = Math.max(0, Math.floor((now - since) / 1000));
		if (s < 60) return 'just now';
		if (s < 3600) return `${Math.floor(s / 60)}m`;
		if (s < 86400) return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
		return `${Math.floor(s / 86400)}d`;
	}
</script>

<main>
	{#if tabs.list.length > 0}
		<nav class="tabs">
			<button class="tab" class:on={showingList} onclick={() => (tabs.active = null)}>＋</button>
			{#each tabs.list as t (t.id)}
				<button class="tab" class:on={tabs.active === t.id} onclick={() => (tabs.active = t.id)}>
					<span class="dot {t.status}"></span>{t.title}
					<span class="x" role="presentation" onclick={(e) => { e.stopPropagation(); tabs.close(t.id); }}>×</span>
				</button>
			{/each}
		</nav>
	{/if}

	<!-- All terminals stay mounted so background tabs keep their SSH sessions. -->
	{#each tabs.list as t (t.id)}
		<Terminal tab={t} active={tabs.active === t.id} onexit={() => tabs.close(t.id)} />
	{/each}

	{#if showingList}
		{#if editing}
			<HostForm host={editing} onsave={save} ondelete={editing.id ? remove : undefined} oncancel={() => (editing = null)} />
		{:else if showSettings}
			<SettingsView onclose={() => (showSettings = false)} />
		{:else if showKeys}
			<KeysView onclose={() => (showKeys = false)} />
		{:else if connecting}
			<ConnectDialog host={connecting} onconnect={connect} oncancel={() => (connecting = null)} />
		{:else}
			<section class="list">
				<header>
					<h1>Hosts</h1>
					<span class="actions">
						<button aria-label="Settings" onclick={() => (showSettings = true)}>⚙</button>
						<button onclick={() => (showKeys = true)}>Keys</button>
						<button class="primary" onclick={() => (editing = {})}>New</button>
					</span>
				</header>
				{#if error}<p class="err">{error}</p>{/if}

				{#if tabs.list.length > 0}
					<h2>Open sessions <span class="count">{tabs.list.length}</span></h2>
					{#each tabs.list as t (t.id)}
						<div class="host session">
							<button class="main" onclick={() => (tabs.active = t.id)}>
								<strong><span class="dot {t.status}"></span>{t.title}</strong>
								<span>
									{t.opts.user}@{t.opts.host}{t.opts.port !== 22 ? ':' + t.opts.port : ''} ·
									{label[t.status]}{t.status === 'ready' && t.since ? ' · ' + age(t.since) : ''}{t.detail && (t.status === 'error' || t.status === 'closed') ? ' · ' + t.detail : ''}
								</span>
							</button>
							<button class="danger" aria-label="Close {t.title}" onclick={() => tabs.close(t.id)}>✕</button>
						</div>
					{/each}
					<h2>Hosts</h2>
				{/if}
				{#each hosts as h (h.id)}
					<div class="host">
						<button class="main" onclick={() => (connecting = h)}>
							<strong>{h.name}</strong>
							<span>{h.user}@{h.host}{h.port !== 22 ? ':' + h.port : ''}</span>
						</button>
						<button aria-label="Edit {h.name}" onclick={() => (editing = h)}>✎</button>
					</div>
				{:else}
					<p class="empty">No hosts yet. Tap <b>New</b> to add your first server.</p>
				{/each}
			</section>
		{/if}
	{/if}
</main>

<style>
	main { display: flex; flex-direction: column; height: 100dvh; padding-top: env(safe-area-inset-top); }
	.tabs { display: flex; gap: 4px; padding: 4px; overflow-x: auto; background: var(--panel); border-bottom: 1px solid var(--line); }
	.tab { flex: 0 0 auto; min-height: 38px; padding: 0.3rem 0.7rem; white-space: nowrap; }
	.tab.on { border-color: var(--accent); }
	.x { margin-left: 0.5rem; color: var(--muted); padding: 0 0.3rem; }
	.list { padding: 16px; max-width: 640px; width: 100%; margin: 0 auto; overflow-y: auto; }
	header { display: flex; justify-content: space-between; align-items: center; }
	h1 { margin: 0; font-size: 1.4rem; }
	h2 { margin: 1.2rem 0 0; font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); }
	.count { background: var(--accent); color: #fff; border-radius: 999px; padding: 0 0.5rem; margin-left: 0.3rem; }
	.dot { display: inline-block; width: 9px; height: 9px; border-radius: 50%; margin-right: 0.5rem; background: var(--muted); }
	.dot.ready { background: var(--ok); }
	.dot.connecting, .dot.verify { background: #d29922; }
	.dot.error, .dot.closed { background: var(--danger); }
	.session .main { border-color: var(--accent); }
	.actions { display: flex; gap: 6px; }
	.host { display: flex; gap: 6px; margin-top: 8px; }
	.host .main { flex: 1; text-align: left; display: flex; flex-direction: column; min-height: 56px; }
	.host span { color: var(--muted); font-size: 0.85rem; }
	.empty { color: var(--muted); margin-top: 2rem; text-align: center; }
	.err { color: var(--danger); }
</style>
