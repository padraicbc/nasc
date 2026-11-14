<script lang="ts">
	import { clearHistory, historyKey, type Host } from './api';

	let {
		host,
		onsave,
		ondelete,
		oncancel
	}: {
		host: Partial<Host>;
		onsave: (h: Partial<Host>) => void;
		ondelete?: () => void;
		oncancel: () => void;
	} = $props();

	// svelte-ignore state_referenced_locally
	let draft = $state({ port: 22, ...host });
</script>

<form
	class="sheet"
	onsubmit={(e) => {
		e.preventDefault();
		onsave({ ...draft, port: Number(draft.port) || 22 });
	}}
>
	<h2>{host.id ? 'Edit host' : 'New host'}</h2>
	<label for="name">Name</label>
	<input id="name" bind:value={draft.name} placeholder="My server" autocapitalize="off" />
	<label for="addr">Address</label>
	<input id="addr" bind:value={draft.host} required placeholder="192.168.1.10 or example.com" autocapitalize="off" autocorrect="off" />
	<div class="two">
		<div>
			<label for="user">Username</label>
			<input id="user" bind:value={draft.user} required autocapitalize="off" autocorrect="off" />
		</div>
		<div>
			<label for="port">Port</label>
			<input id="port" type="number" inputmode="numeric" bind:value={draft.port} min="1" max="65535" />
		</div>
	</div>
	{#if host.id}
		<button
			type="button"
			onclick={async () => {
				if (confirm('Forget all remembered commands for this host?')) {
					await clearHistory(historyKey({ user: host.user ?? '', host: host.host ?? '', port: host.port ?? 22 }));
				}
			}}>Clear command history</button
		>
	{/if}
	<p class="hint">Passwords and key passphrases are asked for at connect time and never saved.</p>
	<div class="row">
		{#if ondelete}<button type="button" class="danger" onclick={ondelete}>Delete</button>{/if}
		<span class="grow"></span>
		<button type="button" onclick={oncancel}>Cancel</button>
		<button class="primary">Save</button>
	</div>
</form>

<style>
	.sheet { padding: 16px; max-width: 520px; margin: 0 auto; }
	h2 { margin: 0 0 0.5rem; }
	.two { display: grid; grid-template-columns: 1fr 100px; gap: 8px; }
	.row { display: flex; gap: 8px; margin-top: 1.2rem; }
	.grow { flex: 1; }
	.hint { color: var(--muted); font-size: 0.85rem; }
</style>
