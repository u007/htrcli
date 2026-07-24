import { describe, expect, it } from "bun:test";
import { computeStitchPlan } from "./stitch";

describe("computeStitchPlan", () => {
	it("sizes the canvas in device pixels and clamps the last row's scroll", () => {
		const plan = computeStitchPlan(1280, 2400, 1280, 720, 2);
		expect(plan.canvasWidth).toBe(2560);
		expect(plan.canvasHeight).toBe(4800);
		expect(plan.segments.map((s) => s.scrollY)).toEqual([0, 720, 1440, 1680]);
		expect(plan.segments.every((s) => s.scrollX === 0)).toBe(true);
	});

	it("handles a page that fits in one viewport (single segment)", () => {
		const plan = computeStitchPlan(800, 600, 800, 600, 1);
		expect(plan.segments).toEqual([{ scrollX: 0, scrollY: 0 }]);
		expect(plan.canvasWidth).toBe(800);
		expect(plan.canvasHeight).toBe(600);
	});

	it("tiles both axes for a page wider and taller than the viewport", () => {
		const plan = computeStitchPlan(2000, 1500, 1000, 1000, 1);
		expect(plan.segments.length).toBe(4);
		expect(plan.segments).toContainEqual({ scrollX: 1000, scrollY: 500 });
	});
});
