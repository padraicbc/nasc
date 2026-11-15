import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [
		sveltekit({
			// Single-page app served by the Go core; unknown routes fall back to index.html.
			adapter: adapter({ pages: 'build', assets: 'build', fallback: 'index.html', strict: false })
		})
	],
	test: {
		include: ['src/**/*.test.ts'],
		coverage: { provider: 'v8', include: ['src/lib/**/*.ts'], reporter: ['text-summary', 'lcov'] }
	}
});
