import { describe, expect, it } from "bun:test";
import { generateId } from "../types/recording";

describe("generateId", () => {
	describe("basic functionality", () => {
		it("should generate a non-empty string", () => {
			const id = generateId();

			expect(id).toBeDefined();
			expect(typeof id).toBe("string");
			expect(id.length).toBeGreaterThan(0);
		});

		it("should generate unique IDs on consecutive calls", () => {
			const ids = new Set<string>();
			const count = 100;

			for (let i = 0; i < count; i++) {
				ids.add(generateId());
			}

			expect(ids.size).toBe(count);
		});

		it("should contain timestamp component", () => {
			const before = Date.now();
			const id = generateId();
			const after = Date.now();

			// Extract numeric part (timestamp) from the ID
			const parts = id.split("_");
			const timestampPart = Number.parseInt(parts[0], 10);

			// If there's a prefix, timestamp will be in second part
			const timestamp =
				Number.isNaN(timestampPart) && parts.length > 1
					? Number.parseInt(parts[1], 10)
					: timestampPart;

			expect(timestamp).toBeGreaterThanOrEqual(before);
			expect(timestamp).toBeLessThanOrEqual(after);
		});
	});

	describe("prefix handling", () => {
		it("should prepend prefix when provided", () => {
			const id = generateId("step_");

			expect(id.startsWith("step_")).toBe(true);
		});

		it("should work with different prefixes", () => {
			const stepId = generateId("step_");
			const sessionId = generateId("session_");
			const annotationId = generateId("ann_");

			expect(stepId.startsWith("step_")).toBe(true);
			expect(sessionId.startsWith("session_")).toBe(true);
			expect(annotationId.startsWith("ann_")).toBe(true);
		});

		it("should generate empty prefix when not provided", () => {
			const id = generateId();

			// ID should start with a number (timestamp)
			expect(id).toMatch(/^\d+/);
		});

		it("should generate empty prefix when empty string provided", () => {
			const id = generateId("");

			// ID should start with a number (timestamp)
			expect(id).toMatch(/^\d+/);
		});
	});

	describe("ID format", () => {
		it("should contain underscore separator", () => {
			const id = generateId();

			expect(id).toContain("_");
		});

		it("should have format: [prefix]timestamp_randomstring", () => {
			const id = generateId("test_");
			const parts = id.split("_");

			expect(parts.length).toBeGreaterThanOrEqual(2);
			expect(parts[0]).toBe("test");
			// Second part should be numeric (timestamp)
			expect(parts[1]).toMatch(/^\d+$/);
			// Third part should be alphanumeric random string
			expect(parts[2]).toMatch(/^[a-z0-9]+$/);
		});

		it("should generate random portion with expected length", () => {
			const id = generateId();
			const parts = id.split("_");
			const randomPart = parts[parts.length - 1];

			// substr(2, 9) should produce at most 9 characters
			expect(randomPart.length).toBeLessThanOrEqual(9);
			expect(randomPart.length).toBeGreaterThan(0);
		});
	});

	describe("uniqueness over time", () => {
		it("should generate different IDs even with same prefix", async () => {
			const id1 = generateId("test_");
			// Small delay to ensure different timestamp
			await new Promise((resolve) => setTimeout(resolve, 1));
			const id2 = generateId("test_");

			expect(id1).not.toBe(id2);
		});

		it("should generate different IDs in rapid succession", () => {
			const ids: string[] = [];
			for (let i = 0; i < 1000; i++) {
				ids.push(generateId());
			}

			const uniqueIds = new Set(ids);
			expect(uniqueIds.size).toBe(1000);
		});
	});
});
