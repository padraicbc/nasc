import { describe, expect, it } from 'vitest';
import { goBack, pushBack } from './nav';

describe('back stack', () => {
	it('runs the most recent action first and reports when empty', () => {
		const calls: string[] = [];
		pushBack(() => calls.push('keys'));
		pushBack(() => calls.push('generate'));
		expect(goBack()).toBe(true);
		expect(goBack()).toBe(true);
		expect(goBack()).toBe(false);
		expect(calls).toEqual(['generate', 'keys']);
	});

	it('removes an action when its screen closes normally', () => {
		const calls: string[] = [];
		const off = pushBack(() => calls.push('form'));
		off();
		expect(goBack()).toBe(false);
		expect(calls).toEqual([]);
	});

	it('cleanup after goBack already ran is harmless', () => {
		const off = pushBack(() => {});
		goBack();
		expect(() => off()).not.toThrow();
	});
});
