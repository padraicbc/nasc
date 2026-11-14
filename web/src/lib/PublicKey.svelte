<script lang="ts">
	let { value, hint = false }: { value: string; hint?: boolean } = $props();
	let copied = $state(false);
	let area: HTMLTextAreaElement;

	async function copy() {
		try {
			await navigator.clipboard.writeText(value);
		} catch {
			area.select(); // fallback: the user can long-press → Copy
			document.execCommand('copy');
		}
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}
</script>

<div class="pub">
	<textarea bind:this={area} readonly rows="3" value={value} spellcheck="false" onfocus={(e) => e.currentTarget.select()}></textarea>
	<button type="button" onclick={copy}>{copied ? 'Copied ✓' : 'Copy public key'}</button>
	{#if hint}
		<p class="hint">Add this line to <code>~/.ssh/authorized_keys</code> on the server, then connect with this key.</p>
	{/if}
</div>

<style>
	textarea { font: 0.75rem ui-monospace, monospace; margin-bottom: 6px; }
	.hint { color: var(--muted); font-size: 0.85rem; }
</style>
