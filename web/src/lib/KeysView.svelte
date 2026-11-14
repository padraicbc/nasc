<script lang="ts">
	import { onMount } from 'svelte';
	import { deleteKey, listKeys, type KeyInfo } from './api';
	import KeyImport from './KeyImport.svelte';
	import KeyGenerate from './KeyGenerate.svelte';
	import PublicKey from './PublicKey.svelte';
	import { pushBack } from './nav';

	let { onclose }: { onclose: () => void } = $props();

	let keys = $state<KeyInfo[]>([]);
	let adding = $state(false);
	let generating = $state(false);
	let error = $state('');

	$effect(() => { if (generating) return pushBack(() => (generating = false)); });
	$effect(() => { if (adding) return pushBack(() => (adding = false)); });

	const refresh = async () => {
		try {
			keys = await listKeys();
		} catch (e) {
			error = (e as Error).message;
		}
	};
	onMount(refresh);

	async function remove(k: KeyInfo) {
		if (!confirm(`Delete key "${k.name}"? Hosts using it will need another key.`)) return;
		await deleteKey(k.id);
		await refresh();
	}
</script>

<section class="sheet">
	<header>
		<h1>SSH keys</h1>
		<button onclick={onclose}>Done</button>
	</header>
	{#if error}<p class="err">{error}</p>{/if}

	{#if generating}
		<KeyGenerate
			ondone={async () => {
				generating = false;
				await refresh();
			}}
			oncancel={() => (generating = false)}
		/>
	{:else if adding}
		<KeyImport
			onsaved={async () => {
				adding = false;
				await refresh();
			}}
			oncancel={() => (adding = false)}
		/>
	{:else}
		<div class="add">
			<button class="primary" onclick={() => (generating = true)}>Generate key</button>
			<button onclick={() => (adding = true)}>Import key</button>
		</div>
		{#each keys as k (k.id)}
			<div class="key">
				<div class="info">
					<strong>{k.name}</strong>
					<span>{k.type ?? 'unknown type'}{k.encrypted ? ' · passphrase' : ''}</span>
					{#if k.fingerprint}<code>{k.fingerprint}</code>{/if}
					{#if k.publicKey}
						<details>
							<summary>Public key</summary>
							<PublicKey value={k.publicKey} />
						</details>
					{/if}
				</div>
				<button class="danger" aria-label="Delete {k.name}" onclick={() => remove(k)}>Delete</button>
			</div>
		{:else}
			<p class="empty">No saved keys. Import one to log in without typing a password.</p>
		{/each}
	{/if}
</section>

<style>
	.sheet { padding: 16px; max-width: 640px; width: 100%; margin: 0 auto; overflow-y: auto; }
	header { display: flex; justify-content: space-between; align-items: center; }
	h1 { margin: 0; font-size: 1.4rem; }
	.add { display: flex; gap: 8px; margin: 12px 0; }
	.add button { flex: 1; }
	details { margin-top: 6px; }
	summary { color: var(--accent); }
	.key { display: flex; gap: 8px; align-items: center; margin-top: 8px; padding: 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); }
	.info { flex: 1; min-width: 0; display: flex; flex-direction: column; }
	.info span { color: var(--muted); font-size: 0.85rem; }
	code { font-size: 0.7rem; color: var(--muted); word-break: break-all; }
	.empty { color: var(--muted); margin-top: 2rem; text-align: center; }
	.err { color: var(--danger); }
</style>
