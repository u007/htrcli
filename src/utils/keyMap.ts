/**
 * CDP key descriptor table.
 *
 * Single source of truth for mapping a key *name* (what `pressKey` receives —
 * e.g. "Enter", "a", "5", " ") to the CDP `Input.dispatchKeyEvent` parameters.
 * Used by both the trusted (CDP) input path and the synthetic-event fallback,
 * so both agree on the `code`/`windowsVirtualKeyCode` values instead of the
 * old hand-built `"Key" + key` strings that produced wrong codes for anything
 * that wasn't a single letter.
 *
 * CDP `Input.dispatchKeyEvent` expects:
 *   - `key`: the printable character or key name ("Enter", "a", " ")
 *   - `code`: the physical-key code ("KeyA", "Digit5", "Enter", "Space", ...)
 *   - `windowsVirtualKeyCode`: the Windows VK value (case-insensitive, so
 *     "A" and "a" share VK 65)
 *   - `text`: the character to insert for printable keys (Enter uses "\r")
 */

export interface CdpKeyDescriptor {
	/** The `key` value (printable char, or key name like "Enter"). */
	key: string;
	/** The `code` value (physical key, e.g. "KeyA", "Digit5", "Enter"). */
	code: string;
	/** Windows virtual key code (case-insensitive, shift-less). */
	windowsVirtualKeyCode: number;
	/** The character to insert for printable keys (undefined for non-printable). */
	text?: string;
	/** True when `text` is present (i.e. the key produces input). */
	isPrintable: boolean;
}

/**
 * Named (non-character) keys. `text` is set for keys that insert a character.
 */
const NAMED_KEYS: Record<string, Omit<CdpKeyDescriptor, "isPrintable">> = {
	Enter: { key: "Enter", code: "Enter", windowsVirtualKeyCode: 13, text: "\r" },
	Tab: { key: "Tab", code: "Tab", windowsVirtualKeyCode: 9, text: "\t" },
	Escape: { key: "Escape", code: "Escape", windowsVirtualKeyCode: 27 },
	Backspace: { key: "Backspace", code: "Backspace", windowsVirtualKeyCode: 8 },
	Delete: { key: "Delete", code: "Delete", windowsVirtualKeyCode: 46 },
	ArrowUp: { key: "ArrowUp", code: "ArrowUp", windowsVirtualKeyCode: 38 },
	ArrowDown: { key: "ArrowDown", code: "ArrowDown", windowsVirtualKeyCode: 40 },
	ArrowLeft: { key: "ArrowLeft", code: "ArrowLeft", windowsVirtualKeyCode: 37 },
	ArrowRight: {
		key: "ArrowRight",
		code: "ArrowRight",
		windowsVirtualKeyCode: 39,
	},
	Home: { key: "Home", code: "Home", windowsVirtualKeyCode: 36 },
	End: { key: "End", code: "End", windowsVirtualKeyCode: 35 },
	PageUp: { key: "PageUp", code: "PageUp", windowsVirtualKeyCode: 33 },
	PageDown: { key: "PageDown", code: "PageDown", windowsVirtualKeyCode: 34 },
};

/**
 * Unshifted symbol characters → their physical code + VK (US QWERTY layout).
 */
const SYMBOL_KEYS: Record<
	string,
	{ code: string; windowsVirtualKeyCode: number }
> = {
	" ": { code: "Space", windowsVirtualKeyCode: 32 },
	"`": { code: "Backquote", windowsVirtualKeyCode: 192 },
	"-": { code: "Minus", windowsVirtualKeyCode: 189 },
	"=": { code: "Equal", windowsVirtualKeyCode: 187 },
	"[": { code: "BracketLeft", windowsVirtualKeyCode: 219 },
	"]": { code: "BracketRight", windowsVirtualKeyCode: 221 },
	"\\": { code: "Backslash", windowsVirtualKeyCode: 220 },
	";": { code: "Semicolon", windowsVirtualKeyCode: 186 },
	"'": { code: "Quote", windowsVirtualKeyCode: 222 },
	",": { code: "Comma", windowsVirtualKeyCode: 188 },
	".": { code: "Period", windowsVirtualKeyCode: 190 },
	"/": { code: "Slash", windowsVirtualKeyCode: 191 },
};

/**
 * Shifted symbol characters → the physical code + VK of the *base* (unshifted)
 * key they share, so the VK stays shift-less and consistent with `resolveKey`
 * of the base character.
 */
const SHIFTED_SYMBOL_KEYS: Record<
	string,
	{ code: string; windowsVirtualKeyCode: number }
> = {
	"~": { code: "Backquote", windowsVirtualKeyCode: 192 },
	_: { code: "Minus", windowsVirtualKeyCode: 189 },
	"+": { code: "Equal", windowsVirtualKeyCode: 187 },
	"{": { code: "BracketLeft", windowsVirtualKeyCode: 219 },
	"}": { code: "BracketRight", windowsVirtualKeyCode: 221 },
	"|": { code: "Backslash", windowsVirtualKeyCode: 220 },
	":": { code: "Semicolon", windowsVirtualKeyCode: 186 },
	'"': { code: "Quote", windowsVirtualKeyCode: 222 },
	"<": { code: "Comma", windowsVirtualKeyCode: 188 },
	">": { code: "Period", windowsVirtualKeyCode: 190 },
	"?": { code: "Slash", windowsVirtualKeyCode: 191 },
	"!": { code: "Digit1", windowsVirtualKeyCode: 49 },
	"@": { code: "Digit2", windowsVirtualKeyCode: 50 },
	"#": { code: "Digit3", windowsVirtualKeyCode: 51 },
	$: { code: "Digit4", windowsVirtualKeyCode: 52 },
	"%": { code: "Digit5", windowsVirtualKeyCode: 53 },
	"^": { code: "Digit6", windowsVirtualKeyCode: 54 },
	"&": { code: "Digit7", windowsVirtualKeyCode: 55 },
	"*": { code: "Digit8", windowsVirtualKeyCode: 56 },
	"(": { code: "Digit9", windowsVirtualKeyCode: 57 },
	")": { code: "Digit0", windowsVirtualKeyCode: 48 },
};

/** Digits 0-9 → "DigitN" code + matching VK. */
function digitDescriptor(digit: string): Omit<CdpKeyDescriptor, "isPrintable"> {
	const vk = digit.charCodeAt(0);
	return {
		key: digit,
		code: `Digit${digit}`,
		windowsVirtualKeyCode: vk,
		text: digit,
	};
}

/** Letters a-z / A-Z → "KeyX" code + shift-less VK (uppercase char code). */
function letterDescriptor(
	letter: string,
): Omit<CdpKeyDescriptor, "isPrintable"> {
	const upper = letter.toUpperCase();
	const vk = upper.charCodeAt(0);
	return {
		key: letter,
		code: `Key${upper}`,
		windowsVirtualKeyCode: vk,
		text: letter,
	};
}

/**
 * Resolve a key name to its CDP descriptor.
 *
 * @param key a named key ("Enter", "Tab", "ArrowDown", ...) or a single
 *   printable character ("a", "5", " ", "@", ...).
 * @throws if the key is not a known name or printable ASCII character.
 */
export function resolveKey(key: string): CdpKeyDescriptor {
	if (!key || typeof key !== "string") {
		throw new Error(
			`resolveKey: empty or non-string key (got ${JSON.stringify(key)})`,
		);
	}

	// 1. Named keys (exact match, case-sensitive).
	if (NAMED_KEYS[key]) {
		return {
			...NAMED_KEYS[key],
			isPrintable: NAMED_KEYS[key].text !== undefined,
		};
	}

	// 2. Single printable character.
	if (key.length === 1) {
		const code = key.charCodeAt(0);
		// Letters
		if ((code >= 65 && code <= 90) || (code >= 97 && code <= 122)) {
			return { ...letterDescriptor(key), isPrintable: true };
		}
		// Digits
		if (code >= 48 && code <= 57) {
			return { ...digitDescriptor(key), isPrintable: true };
		}
		// Space
		if (code === 32) {
			return {
				key: " ",
				code: "Space",
				windowsVirtualKeyCode: 32,
				text: " ",
				isPrintable: true,
			};
		}
		// Unshifted symbols
		if (SYMBOL_KEYS[key]) {
			return {
				key,
				code: SYMBOL_KEYS[key].code,
				windowsVirtualKeyCode: SYMBOL_KEYS[key].windowsVirtualKeyCode,
				text: key,
				isPrintable: true,
			};
		}
		// Shifted symbols (share the base key's code/VK)
		if (SHIFTED_SYMBOL_KEYS[key]) {
			return {
				key,
				code: SHIFTED_SYMBOL_KEYS[key].code,
				windowsVirtualKeyCode: SHIFTED_SYMBOL_KEYS[key].windowsVirtualKeyCode,
				text: key,
				isPrintable: true,
			};
		}
	}

	throw new Error(`resolveKey: unknown key "${key}"`);
}
