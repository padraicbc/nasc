<script lang="ts">
	import { onMount } from 'svelte';

	let { onclose }: { onclose: () => void } = $props();

	// Present only inside the Android app (see MainActivity.LockBridge).
	const native = (window as any).NascNative as
		| { lockSupported(): boolean; lockEnabled(): boolean; setLock(on: boolean): void }
		| undefined;

	let enabled = $state(native?.lockEnabled() ?? false);
	let busy = $state(false);
	let error = $state('');
	const supported = native?.lockSupported() ?? false;

	onMount(() => {
		const handler = (e: Event) => {
			const d = (e as CustomEvent).detail as { enabled: boolean; error: string | null };
			enabled = d.enabled;
			error = d.error ?? '';
			busy = false;
		};
		window.addEventListener('nasc-lock', handler);
		return () => window.removeEventListener('nasc-lock', handler);
	});

	function toggle() {
		busy = true;
		error = '';
		native!.setLock(!enabled);
	}
</script>

<section class="sheet">
	<header>
		<h1>Settings</h1>
		<button onclick={onclose}>Done</button>
	</header>

	<h2>Security</h2>
	{#if !native}
		<p class="hint">App lock is available in the Android app.</p>
	{:else if !supported}
		<p class="hint">
			Set up a fingerprint, face or screen lock in Android settings to use app lock.
		</p>
	{:else}
		<div class="row">
			<div>
				<strong>App lock</strong>
				<p class="hint">
					Require fingerprint, face or your screen lock to open Nasc. Saved SSH keys can't be
					decrypted without it, and it's asked again after the app has been away for 30 seconds.
					Open sessions keep running in the background.
				</p>
			</div>
			<button class:primary={!enabled} disabled={busy} onclick={toggle}>
				{enabled ? 'Turn off' : 'Turn on'}
			</button>
		</div>
		<p class="status">{enabled ? '🔒 Lock is on' : 'Lock is off'}</p>
	{/if}
	{#if error}<p class="err">{error}</p>{/if}
</section>

<style>
	.sheet { padding: 16px; max-width: 640px; width: 100%; margin: 0 auto; }
	header { display: flex; justify-content: space-between; align-items: center; }
	h1 { margin: 0; font-size: 1.4rem; }
	h2 { font-size: 1rem; color: var(--muted); margin: 1.5rem 0 0.5rem; }
	.row { display: flex; gap: 12px; align-items: flex-start; justify-content: space-between; }
	.row p { margin: 0.2rem 0 0; }
	.hint { color: var(--muted); font-size: 0.85rem; }
	.status { margin-top: 0.8rem; }
	.err { color: var(--danger); }
</style>
