// Autocomplete for the terminal. The shell runs on the remote end, so we can't read its
// command line; instead we follow what the user types and give up (until Enter) whenever
// the line could have been changed behind our back (cursor keys, remote Tab completion...).

export class LineTracker {
	line = '';
	/** False once the line may differ from what we typed; suggestions are hidden until Enter. */
	known = true;

	/** Feed raw terminal input. Returns the commands the user submitted with Enter. */
	feed(data: string): string[] {
		const submitted: string[] = [];
		for (const ch of data) {
			switch (ch) {
				case '\x1b': // arrows, Home/End, Alt+key...: an escape sequence, ignore the rest of the chunk
					this.known = false;
					return submitted;
				case '\r':
				case '\n':
					if (this.known && this.line.trim()) submitted.push(this.line);
					this.line = '';
					this.known = true;
					break;
				case '\x7f':
				case '\b':
					this.line = Array.from(this.line).slice(0, -1).join('');
					break;
				case '\x03': // Ctrl+C
				case '\x04': // Ctrl+D
				case '\x15': // Ctrl+U
					this.line = '';
					this.known = true;
					break;
				case '\x17': // Ctrl+W
					this.line = this.line.replace(/\S+\s*$/, '');
					break;
				case '\x0c': // Ctrl+L: redraws only
					break;
				default:
					if (ch < ' ') this.known = false; // Tab and other control keys
					else if (this.known) this.line += ch;
			}
		}
		return submitted;
	}

	/** Pretend the line is now `text` (after the user accepted a suggestion). */
	set(text: string) {
		this.line = text;
		this.known = true;
	}
}

/** Prefix matches from history, most recent first, never the exact line itself. */
export function suggest(history: string[], line: string, max = 5): string[] {
	if (!line.trim()) return [];
	const out: string[] = [];
	const seen = new Set<string>();
	for (const h of history) {
		if (h.length > line.length && h.startsWith(line) && !seen.has(h)) {
			seen.add(h);
			out.push(h);
			if (out.length >= max) break;
		}
	}
	return out;
}

/** The text to send to the remote to turn `line` into `suggestion`. */
export const completion = (line: string, suggestion: string) => suggestion.slice(line.length);

/** Most recent output, used to avoid recording what is typed at a password prompt. */
export class OutputTail {
	private text = '';
	private decoder = new TextDecoder();

	push(bytes: Uint8Array) {
		this.text += this.decoder.decode(bytes, { stream: true });
		if (this.text.length > 300) this.text = this.text.slice(-300);
	}

	atSecretPrompt(): boolean {
		// eslint-disable-next-line no-control-regex
		const plain = this.text.replace(/\x1b\[[0-9;?]*[ -/]*[@-~]/g, '').replace(/\r/g, '');
		const lastLine = plain.split('\n').pop() ?? '';
		return /(password|passphrase|passcode|verification code|token|secret)[^\n]*[:?]\s*$/i.test(lastLine);
	}
}

/** Remember a command locally (newest first, deduped, capped) so suggestions update at once. */
export function remember(history: string[], cmd: string, cap = 500): string[] {
	return [cmd, ...history.filter((h) => h !== cmd)].slice(0, cap);
}

// --- Reading the command line from the terminal screen -------------------------------------
// Keystroke tracking breaks with phone keyboards (autocorrect/composition send text in odd
// chunks) and with anything the shell edits itself (history recall, Tab completion). The screen
// is the truth: remember where the prompt ended, and read everything from there to the cursor.

export interface ScreenLine {
	isWrapped: boolean; // this row continues the previous one
	translateToString(trimRight?: boolean, start?: number, end?: number): string;
}
export interface ScreenBuffer {
	getLine(y: number): ScreenLine | undefined;
	cursorX: number;
	cursorY: number;
	baseY: number;
}
export interface Anchor {
	row: number; // absolute buffer row
	x: number;
}

export const anchorAt = (b: ScreenBuffer): Anchor => ({ row: b.baseY + b.cursorY, x: b.cursorX });

/** Text between the anchor and the cursor, or null if it can't be trusted (cursor mid-line, scrolled away). */
export function readCommand(b: ScreenBuffer, a: Anchor): string | null {
	const endRow = b.baseY + b.cursorY;
	if (endRow < a.row) return null;
	const cur = b.getLine(endRow);
	if (!cur || cur.translateToString(true).trimEnd().length > b.cursorX) return null; // text right of the cursor
	let out = '';
	for (let r = a.row; r <= endRow; r++) {
		const l = b.getLine(r);
		if (!l) return null;
		if (r > a.row && !l.isWrapped) return null; // a real line break: not one command line, don't trust it
		out += l.translateToString(false, r === a.row ? a.x : 0, r === endRow ? b.cursorX : undefined);
	}
	return out;
}
