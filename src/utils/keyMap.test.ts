import { describe, expect, it } from "bun:test";
import { resolveKey, resolveKeySpec } from "./keyMap";

describe("resolveKey (CDP key descriptor table)", () => {
	it("maps Enter to code 'Enter' with VK 13 and text '\\r'", () => {
		const d = resolveKey("Enter");
		expect(d.code).toBe("Enter");
		expect(d.windowsVirtualKeyCode).toBe(13);
		expect(d.text).toBe("\r");
		expect(d.isPrintable).toBe(true);
		expect(d.key).toBe("Enter");
	});

	it("maps letters to KeyX codes with shift-less VKs", () => {
		const lower = resolveKey("a");
		expect(lower.code).toBe("KeyA");
		expect(lower.windowsVirtualKeyCode).toBe(65);
		expect(lower.text).toBe("a");
		expect(lower.isPrintable).toBe(true);

		// Uppercase letter shares the shift-less VK but keeps its own key text.
		const upper = resolveKey("Z");
		expect(upper.code).toBe("KeyZ");
		expect(upper.windowsVirtualKeyCode).toBe(90);
		expect(upper.text).toBe("Z");
	});

	it("maps digits to DigitN-style codes", () => {
		const five = resolveKey("5");
		expect(five.code).toBe("Digit5");
		expect(five.windowsVirtualKeyCode).toBe(53);
		expect(five.text).toBe("5");
		expect(five.isPrintable).toBe(true);
	});

	it("maps space to 'Space'", () => {
		const space = resolveKey(" ");
		expect(space.code).toBe("Space");
		expect(space.windowsVirtualKeyCode).toBe(32);
		expect(space.text).toBe(" ");
		expect(space.key).toBe(" ");
	});

	it("maps ArrowDown to VK 40 and marks it non-printable", () => {
		const down = resolveKey("ArrowDown");
		expect(down.windowsVirtualKeyCode).toBe(40);
		expect(down.isPrintable).toBe(false);
		expect(down.text).toBeUndefined();
	});

	it("maps a shifted symbol to the base key's code and VK", () => {
		const bang = resolveKey("!");
		expect(bang.code).toBe("Digit1");
		expect(bang.windowsVirtualKeyCode).toBe(49);
		expect(bang.text).toBe("!");
		expect(bang.isPrintable).toBe(true);
	});

	it("maps an unshifted symbol to its own code and VK", () => {
		const slash = resolveKey("/");
		expect(slash.code).toBe("Slash");
		expect(slash.windowsVirtualKeyCode).toBe(191);
		expect(slash.text).toBe("/");
		expect(slash.isPrintable).toBe(true);
	});

	it("throws on an unknown key name", () => {
		expect(() => resolveKey("NotAKey")).toThrow(/unknown key/i);
		expect(() => resolveKey("")).toThrow(/empty or non-string/i);
	});
});

describe("resolveKeySpec (modifier combos and function keys)", () => {
	it("leaves a plain key unmodified", () => {
		const spec = resolveKeySpec("Enter");
		expect(spec.modifiers).toBe(0);
		expect(spec.descriptor.code).toBe("Enter");
	});

	it("parses Control+a into the letter plus the Ctrl bit and no inserted text", () => {
		const spec = resolveKeySpec("Control+a");
		expect(spec.modifiers).toBe(2);
		expect(spec.descriptor.key).toBe("a");
		expect(spec.descriptor.code).toBe("KeyA");
		expect(spec.descriptor.text).toBeUndefined();
	});

	it("accepts the Ctrl/Cmd aliases and combines modifiers additively", () => {
		expect(resolveKeySpec("Ctrl+c").modifiers).toBe(2);
		expect(resolveKeySpec("Cmd+a").modifiers).toBe(4);
		expect(resolveKeySpec("Meta+Shift+z").modifiers).toBe(12);
		expect(resolveKeySpec("Alt+ArrowLeft").modifiers).toBe(1);
		// Option/Command are documented aliases for Alt/Meta.
		expect(resolveKeySpec("Option+a").modifiers).toBe(1);
		expect(resolveKeySpec("Command+a").modifiers).toBe(4);
		// Bare Ctrl/Cmd resolve too, matching Go's named-key table.
		expect(resolveKeySpec("Ctrl").descriptor.code).toBe("ControlLeft");
		expect(resolveKeySpec("Cmd").descriptor.code).toBe("MetaLeft");
	});

	it("keeps the character for a Shift-only combo", () => {
		const spec = resolveKeySpec("Shift+a");
		expect(spec.modifiers).toBe(8);
		expect(spec.descriptor.key).toBe("A");
		expect(spec.descriptor.text).toBe("A");
	});

	it("applies Shift to the key even alongside another modifier", () => {
		// A real Ctrl+Shift+A keydown has key "A"; Ctrl suppresses the character
		// but must not revert the letter.
		const spec = resolveKeySpec("Control+Shift+a");
		expect(spec.modifiers).toBe(10);
		expect(spec.descriptor.key).toBe("A");
		expect(spec.descriptor.text).toBeUndefined();
	});

	it("maps Shift+digit and Shift+symbol to the shifted character", () => {
		// Upper-casing a digit is a no-op; the base key must map to what Shift
		// actually produces on the US layout.
		const one = resolveKeySpec("Shift+1");
		expect(one.modifiers).toBe(8);
		expect(one.descriptor.key).toBe("!");
		expect(one.descriptor.text).toBe("!");
		expect(one.descriptor.code).toBe("Digit1");
		expect(one.descriptor.windowsVirtualKeyCode).toBe(49);

		const dash = resolveKeySpec("Shift+-");
		expect(dash.modifiers).toBe(8);
		expect(dash.descriptor.key).toBe("_");
		expect(dash.descriptor.text).toBe("_");
		expect(dash.descriptor.code).toBe("Minus");
	});

	it("supports named keys in combos without inventing a character", () => {
		const spec = resolveKeySpec("Shift+Tab");
		expect(spec.modifiers).toBe(8);
		expect(spec.descriptor.code).toBe("Tab");
		expect(spec.descriptor.key).toBe("Tab");

		// Ctrl/Alt/Meta combinations suppress the character a *single* key would
		// insert, but named keys keep theirs: Ctrl+Enter still sends "\r".
		expect(resolveKeySpec("Ctrl+Enter").descriptor.text).toBe("\r");
	});

	it("maps F1-F12 to their virtual key codes", () => {
		expect(resolveKeySpec("F5").descriptor.windowsVirtualKeyCode).toBe(116);
		expect(resolveKeySpec("F10").descriptor.code).toBe("F10");
		expect(resolveKeySpec("Shift+F10").modifiers).toBe(8);
	});

	it("resolves a bare modifier so it can be held with keyDown", () => {
		expect(resolveKeySpec("Shift").descriptor.code).toBe("ShiftLeft");
		expect(resolveKeySpec("Control").descriptor.windowsVirtualKeyCode).toBe(17);
	});

	it("rejects an unknown key or modifier inside a combo", () => {
		expect(() => resolveKeySpec("Control+nope")).toThrow(/unknown key/i);
		expect(() => resolveKeySpec("Hyper+a")).toThrow(/unknown modifier/i);
	});

	it("treats a trailing + as the plus key, not a dangling modifier", () => {
		// "+" is itself both the separator and a key. `press "+"` must keep
		// working, and `Ctrl++` is Ctrl held with it (zoom in).
		const plus = resolveKeySpec("+");
		expect(plus.modifiers).toBe(8);
		expect(plus.descriptor.key).toBe("+");
		expect(plus.descriptor.code).toBe("Equal");

		const ctrlPlus = resolveKeySpec("Ctrl++");
		expect(ctrlPlus.modifiers).toBe(2);
		expect(ctrlPlus.descriptor.key).toBe("+");
		expect(ctrlPlus.descriptor.code).toBe("Equal");
	});

	it("still rejects a lone trailing + after a modifier", () => {
		expect(() => resolveKeySpec("Ctrl+")).toThrow(/names no key/i);
	});

	it("trims whitespace around the key segment", () => {
		const spec = resolveKeySpec("Ctrl + a");
		expect(spec.modifiers).toBe(2);
		expect(spec.descriptor.key).toBe("a");
	});

	it("derives the Shift bit for a character that requires it", () => {
		// `press "A"`/`press "!"` must not claim shiftKey:false.
		expect(resolveKeySpec("A").modifiers).toBe(8);
		expect(resolveKeySpec("!").modifiers).toBe(8);
		// An explicit modifier spec stays authoritative.
		expect(resolveKeySpec("Ctrl+A").modifiers).toBe(2);
	});
});
