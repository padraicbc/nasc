import { describe, expect, it } from 'vitest';
import { ctrlChar } from './ssh';

describe('ctrlChar', () => {
	it('maps letters to control codes', () => {
		expect(ctrlChar('c')).toBe('\x03');
		expect(ctrlChar('D')).toBe('\x04');
		expect(ctrlChar('[')).toBe('\x1b');
	});
	it('ignores multi-char input and unmappable characters', () => {
		expect(ctrlChar('ab')).toBeNull();
		expect(ctrlChar('1')).toBeNull();
		expect(ctrlChar('')).toBeNull();
	});
});
