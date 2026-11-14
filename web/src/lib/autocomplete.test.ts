import { describe, expect, it } from 'vitest';
import { LineTracker, OutputTail, anchorAt, completion, readCommand, remember, suggest, type ScreenBuffer } from './autocomplete';

describe('LineTracker', () => {
	it('follows typing, backspace and Enter', () => {
		const t = new LineTracker();
		expect(t.feed('ls -lx')).toEqual([]);
		t.feed('\x7f');
		expect(t.line).toBe('ls -l');
		expect(t.feed('a\r')).toEqual(['ls -la']);
		expect(t.line).toBe('');
	});

	it('handles pasted text with several commands', () => {
		const t = new LineTracker();
		expect(t.feed('cd /tmp\rpwd\r')).toEqual(['cd /tmp', 'pwd']);
	});

	it('goes unknown on escape sequences and recovers after Enter, without recording', () => {
		const t = new LineTracker();
		t.feed('git sta');
		t.feed('\x1b[A'); // Up arrow: shell replaced the line
		expect(t.known).toBe(false);
		expect(t.feed('\r')).toEqual([]);
		expect(t.known).toBe(true);
	});

	it('goes unknown on Tab (remote completion) but not on Ctrl+C', () => {
		const t = new LineTracker();
		t.feed('doc\t');
		expect(t.known).toBe(false);
		t.feed('\x03');
		expect(t.known).toBe(true);
		expect(t.line).toBe('');
	});

	it('supports Ctrl+W, Ctrl+U and set()', () => {
		const t = new LineTracker();
		t.feed('echo hello world');
		t.feed('\x17');
		expect(t.line).toBe('echo hello ');
		t.feed('\x15');
		expect(t.line).toBe('');
		t.set('ls');
		expect(t.line).toBe('ls');
	});

	it('handles multi-byte characters on backspace', () => {
		const t = new LineTracker();
		t.feed('echo 😀');
		t.feed('\x7f');
		expect(t.line).toBe('echo ');
	});
});

describe('suggest', () => {
	const history = ['docker ps -a', 'docker compose up', 'ls', 'docker ps -a', 'git status'];
	it('returns prefix matches, newest first, deduped', () => {
		expect(suggest(history, 'docker')).toEqual(['docker ps -a', 'docker compose up']);
	});
	it('excludes the exact line and ignores empty input', () => {
		expect(suggest(history, 'ls')).toEqual([]);
		expect(suggest(history, '  ')).toEqual([]);
	});
	it('limits results', () => {
		expect(suggest(history, 'd', 1)).toEqual(['docker ps -a']);
	});
	it('computes what to send to complete a line', () => {
		expect(completion('docker p', 'docker ps -a')).toBe('s -a');
	});
});

describe('OutputTail', () => {
	const enc = (s: string) => new TextEncoder().encode(s);
	it('spots password prompts', () => {
		const t = new OutputTail();
		t.push(enc('last login...\r\n[sudo] password for bob: '));
		expect(t.atSecretPrompt()).toBe(true);
		t.push(enc('\r\n$ '));
		expect(t.atSecretPrompt()).toBe(false);
	});
	it('ignores colour codes', () => {
		const t = new OutputTail();
		t.push(enc('\x1b[1mEnter passphrase for key:\x1b[0m '));
		expect(t.atSecretPrompt()).toBe(true);
	});
});

describe('remember', () => {
	it('moves repeats to the front and caps the list', () => {
		expect(remember(['a', 'b', 'c'], 'b')).toEqual(['b', 'a', 'c']);
		expect(remember(['a', 'b'], 'c', 2)).toEqual(['c', 'a']);
	});
});

describe('readCommand', () => {
	const mk = (rows: string[], cursorY: number, cursorX: number, baseY = 0, wrapped: number[] = []): ScreenBuffer => ({
		baseY,
		cursorX,
		cursorY,
		getLine: (y) =>
			rows[y] === undefined
				? undefined
				: {
						isWrapped: wrapped.includes(y),
						translateToString: (trim = false, start = 0, end?: number) => {
							const t = (trim ? rows[y].trimEnd() : rows[y].padEnd(20)).slice(start, end);
							return t;
						}
					}
	});

	it('reads what was typed after the prompt, whatever the prompt looks like', () => {
		const prompt = mk(['me@box:~$ '], 0, 10);
		const a = anchorAt(prompt);
		expect(readCommand(mk(['me@box:~$ ech'], 0, 13), a)).toBe('ech');
		expect(readCommand(mk(['me@box:~$ ech'], 0, 13), a)).toBe('ech');
	});

	it('follows a line that the shell edited itself (history recall, remote Tab)', () => {
		const a = { row: 0, x: 10 };
		expect(readCommand(mk(['me@box:~$ docker ps -a'], 0, 22), a)).toBe('docker ps -a');
	});

	it('joins wrapped rows and uses absolute rows when scrolled', () => {
		const a = { row: 3, x: 2 };
		const b = mk(['', '', '', '$ aaaaaaaa', 'bbb'], 1, 3, 3, [4]);
		expect(readCommand(b, a)).toBe('aaaaaaaa'.padEnd(18) + 'bbb');
	});

	it('refuses to join rows that are separate lines (e.g. typed before the prompt appeared)', () => {
		const b = mk(['Last login: Tue', 'me@box:~$ echo hi'], 1, 17);
		expect(readCommand(b, { row: 0, x: 0 })).toBeNull();
	});

	it('ignores blanks right of the cursor (readline erases with real spaces)', () => {
		expect(readCommand(mk(['$ echo second    '], 0, 13), { row: 0, x: 2 })).toBe('echo second');
	});

	it('refuses when the cursor is in the middle of the line', () => {
		expect(readCommand(mk(['$ ls -la'], 0, 4), { row: 0, x: 2 })).toBeNull();
	});
});
