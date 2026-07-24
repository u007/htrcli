import { describe, expect, it } from "bun:test";
import { type AnnotationBox, toAnnotationBox } from "./highlighter";

describe("toAnnotationBox", () => {
	it("converts a viewport rect to document-absolute coordinates", () => {
		const rect = { left: 10, top: 20, width: 30, height: 40 } as DOMRect;
		const box: AnnotationBox = toAnnotationBox(rect, 0, 100, 1);
		expect(box).toEqual({ number: 1, x: 10, y: 120, width: 30, height: 40 });
	});

	it("applies horizontal scroll offset too", () => {
		const rect = { left: 5, top: 5, width: 8, height: 8 } as DOMRect;
		const box = toAnnotationBox(rect, 50, 0, 7);
		expect(box.x).toBe(55);
		expect(box.number).toBe(7);
	});
});
