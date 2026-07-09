import { describe, expect, it } from "bun:test";
import { resolveKey } from "./keyMap";

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
