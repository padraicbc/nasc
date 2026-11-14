// Back navigation. The app is state-driven (no URLs per screen), so each sub-screen registers a
// "go back" action while it is showing. Android's back button calls window.nascBack(): if an action
// is registered it runs (and we return true); otherwise we are at the top and the native side
// sends the app to the background instead of closing it, so sessions stay alive.

const stack: (() => void)[] = [];

/** Register a back action; returns a function that removes it (use as an $effect cleanup). */
export function pushBack(fn: () => void): () => void {
	stack.push(fn);
	return () => {
		const i = stack.lastIndexOf(fn);
		if (i >= 0) stack.splice(i, 1);
	};
}

/** Run the most recent back action. False when there is nothing to go back to. */
export function goBack(): boolean {
	const fn = stack.pop();
	if (!fn) return false;
	fn();
	return true;
}

if (typeof window !== 'undefined') (window as unknown as { nascBack: () => boolean }).nascBack = goBack;
