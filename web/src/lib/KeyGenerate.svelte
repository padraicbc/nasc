<script lang="ts">
	import { generateKey, type KeyInfo } from './api';
	import PublicKey from './PublicKey.svelte';

	let { ondone, oncancel }: { ondone: (k: KeyInfo) => void; oncancel: () => void } = $props();

	let name = $state('');
	let type = $state<'ed25519' | 'rsa'>('ed25519');
	let passphrase = $state('');
	let busy = $state(false);
	let error = $state('');
	let made = $state<KeyInfo | null>(null);

	async function make() {
		busy = true;
		error = '';
		try {
			made = await generateKey(name, type, passphrase);
			passphrase = '';
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy = false;
		}
	}
</script>

{#if made}
	<h2>Key created</h2>
	<p><strong>{made.name}</strong> · {made.type}</p>
	<p class="fp">{made.fingerprint}</p>
	{#if made.publicKey}<PublicKey value={made.publicKey} hint />{/if}
	<p class="hint">The private key stays encrypted on this device and can't be shown or exported.</p>
	<div class="row"><button class="primary" onclick={() => ondone(made!)}>Done</button></div>
{:else}
	<form onsubmit={(e) => { e.preventDefault(); make(); }}>
		<h2>Generate key</h2>
		<label for="gname">Name</label>
		<input id="gname" bind:value={name} placeholder="e.g. my phone" autocapitalize="off" />
		<label for="gtype">Type</label>
		<select id="gtype" bind:value={type}>
			<option value="ed25519">Ed25519 (recommended, fast)</option>
			<option value="rsa">RSA 4096 (older servers; takes a few seconds)</option>
		</select>
		<label for="gpass">Passphrase (optional)</label>
		<input id="gpass" type="password" bind:value={passphrase} autocomplete="off" />
		<p class="hint">With a passphrase you'll type it each time you connect, in addition to the device protection.</p>
		{#if error}<p class="err">{error}</p>{/if}
		<div class="row">
			<button type="button" onclick={oncancel}>Cancel</button>
			<button class="primary" disabled={busy}>{busy ? 'Generating…' : 'Generate'}</button>
		</div>
	</form>
{/if}

<style>
	h2 { margin: 0 0 0.5rem; }
	.hint { color: var(--muted); font-size: 0.85rem; }
	.fp { font: 0.75rem ui-monospace, monospace; color: var(--muted); word-break: break-all; }
	.err { color: var(--danger); }
	.row { display: flex; gap: 8px; justify-content: flex-end; margin-top: 1rem; }
</style>
