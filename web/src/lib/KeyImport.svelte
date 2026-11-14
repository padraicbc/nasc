<script lang="ts">
	import { addKey, type KeyInfo } from './api';

	let { onsaved, oncancel }: { onsaved: (k: KeyInfo) => void; oncancel?: () => void } = $props();

	let name = $state('');
	let privateKey = $state('');
	let error = $state('');
	let busy = $state(false);

	async function pickFile(e: Event) {
		const f = (e.target as HTMLInputElement).files?.[0];
		if (!f) return;
		privateKey = await f.text();
		if (!name) name = f.name.replace(/\.(pem|key)$/, '');
	}

	async function save() {
		busy = true;
		error = '';
		try {
			onsaved(await addKey(name, privateKey));
			privateKey = ''; // don't keep the secret in the page any longer than needed
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy = false;
		}
	}
</script>

<form
	onsubmit={(e) => {
		e.preventDefault();
		save();
	}}
>
	<label for="kname">Name</label>
	<input id="kname" bind:value={name} placeholder="e.g. work laptop" autocapitalize="off" />
	<label for="kkey">Private key (PEM / OpenSSH)</label>
	<textarea id="kkey" rows="6" bind:value={privateKey} spellcheck="false" required placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"></textarea>
	<input type="file" onchange={pickFile} />
	<p class="hint">
		Stored encrypted on this device, protected by the Android Keystore. The key never goes back to the
		screen once saved. A key passphrase is not stored; you'll be asked for it when connecting.
	</p>
	{#if error}<p class="err">{error}</p>{/if}
	<div class="row">
		{#if oncancel}<button type="button" onclick={oncancel}>Cancel</button>{/if}
		<button class="primary" disabled={busy}>Save key</button>
	</div>
</form>

<style>
	textarea { font: 0.8rem ui-monospace, monospace; margin-bottom: 6px; }
	.hint { color: var(--muted); font-size: 0.85rem; }
	.err { color: var(--danger); }
	.row { display: flex; gap: 8px; justify-content: flex-end; margin-top: 1rem; }
</style>
