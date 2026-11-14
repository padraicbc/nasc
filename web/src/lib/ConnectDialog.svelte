<script lang="ts">
	import { onMount } from 'svelte';
	import { listKeys, type Host, type KeyInfo } from './api';
	import KeyImport from './KeyImport.svelte';
	import { pushBack } from './nav';

	let {
		host,
		onconnect,
		oncancel
	}: {
		host: Host;
		onconnect: (secrets: { password?: string; keyId?: string; passphrase?: string }) => void;
		oncancel: () => void;
	} = $props();

	let mode = $state<'password' | 'key'>('password');
	let password = $state('');
	let passphrase = $state('');
	let keys = $state<KeyInfo[]>([]);
	let keyId = $state('');
	let importing = $state(false);

	$effect(() => { if (importing) return pushBack(() => (importing = false)); });

	const selected = $derived(keys.find((k) => k.id === keyId));

	onMount(async () => {
		try {
			keys = await listKeys();
			if (keys.length) {
				keyId = keys[0].id;
				mode = 'key';
			}
		} catch {
			/* no saved keys: password mode */
		}
	});
</script>

<form
	class="sheet"
	onsubmit={(e) => {
		e.preventDefault();
		onconnect(mode === 'password' ? { password } : { keyId, passphrase: passphrase || undefined });
	}}
>
	<h2>{host.name}</h2>
	<p class="hint">{host.user}@{host.host}:{host.port}</p>

	<div class="seg">
		<button type="button" class:on={mode === 'password'} onclick={() => (mode = 'password')}>Password</button>
		<button type="button" class:on={mode === 'key'} onclick={() => (mode = 'key')}>SSH key</button>
	</div>

	{#if mode === 'password'}
		<label for="pw">Password</label>
		<!-- svelte-ignore a11y_autofocus -->
		<input id="pw" type="password" bind:value={password} autofocus autocomplete="off" required />
	{:else if importing}
		<KeyImport
			onsaved={(k) => {
				keys = [...keys, k];
				keyId = k.id;
				importing = false;
			}}
			oncancel={() => (importing = false)}
		/>
	{:else}
		<label for="saved">Key</label>
		<select id="saved" bind:value={keyId} required>
			{#each keys as k (k.id)}
				<option value={k.id}>{k.name} ({k.type ?? 'key'})</option>
			{/each}
		</select>
		<button type="button" class="link" onclick={() => (importing = true)}>＋ Import a new key</button>
		{#if selected?.encrypted}
			<label for="pp">Passphrase</label>
			<input id="pp" type="password" bind:value={passphrase} autocomplete="off" required />
		{:else}
			<label for="pp">Passphrase (only if the key has one)</label>
			<input id="pp" type="password" bind:value={passphrase} autocomplete="off" />
		{/if}
	{/if}

	{#if !importing}
		<div class="row">
			<button type="button" onclick={oncancel}>Cancel</button>
			<button class="primary" disabled={mode === 'key' && !keyId}>Connect</button>
		</div>
	{/if}
</form>

<style>
	.sheet { padding: 16px; max-width: 520px; margin: 0 auto; }
	h2 { margin: 0; }
	.hint { color: var(--muted); margin: 0.2rem 0 1rem; }
	.seg { display: flex; gap: 6px; }
	.seg button { flex: 1; }
	.seg button.on { border-color: var(--accent); background: #2f81f722; }
	.row { display: flex; gap: 8px; justify-content: flex-end; margin-top: 1.2rem; }
	.link { border: none; background: none; color: var(--accent); padding: 0.4rem 0; min-height: 0; }
</style>
