import { describe, expect, it } from "bun:test";
import {
	sampleEmptySession,
	sampleFullSession,
	sampleMinimalSession,
	sampleSessionWithSpecialChars,
} from "../test/fixtures";
import { exportToJson } from "../utils/exportJson";

describe("exportToJson", () => {
	describe("basic functionality", () => {
		it("should export an empty session correctly", () => {
			const result = exportToJson(sampleEmptySession);

			expect(result.id).toBe(sampleEmptySession.id);
			expect(result.title).toBe(sampleEmptySession.title);
			expect(result.startTime).toBe(sampleEmptySession.startTime);
			expect(result.endTime).toBe(sampleEmptySession.endTime);
			expect(result.hasAudio).toBe(false);
			expect(result.steps).toEqual([]);
			expect(result.annotations).toEqual([]);
		});

		it("should export a minimal session with navigation and click steps", () => {
			const result = exportToJson(sampleMinimalSession);

			expect(result.steps).toHaveLength(2);
			expect(result.steps[0].type).toBe("navigation");
			expect(result.steps[1].type).toBe("click");
		});

		it("should preserve session metadata", () => {
			const result = exportToJson(sampleFullSession);

			expect(result.id).toBe(sampleFullSession.id);
			expect(result.title).toBe(sampleFullSession.title);
			expect(result.startTime).toBe(sampleFullSession.startTime);
			expect(result.endTime).toBe(sampleFullSession.endTime);
			expect(result.hasAudio).toBe(true);
		});
	});

	describe("screenshot path generation", () => {
		it("should generate correct screenshot paths for steps with screenshots", () => {
			const result = exportToJson(sampleMinimalSession);

			expect(result.steps[0].screenshotPath).toBe("screenshots/step_1.png");
			expect(result.steps[1].screenshotPath).toBe("screenshots/step_2.png");
		});

		it("should not include screenshotPath when step has no screenshot", () => {
			const sessionWithNoScreenshots = {
				...sampleMinimalSession,
				steps: [
					{ ...sampleMinimalSession.steps[0], screenshotData: undefined },
				],
			};
			const result = exportToJson(sessionWithNoScreenshots);

			expect(result.steps[0].screenshotPath).toBeUndefined();
		});

		it("should generate correct screenshot paths for annotations", () => {
			const result = exportToJson(sampleFullSession);

			// First annotation has no screenshot, second has screenshot
			expect(result.annotations[0].screenshotPath).toBeUndefined();
			expect(result.annotations[1].screenshotPath).toBe(
				"screenshots/annotation_2.png",
			);
		});
	});

	describe("audio path generation", () => {
		it("should generate correct audio paths for steps with audio", () => {
			const result = exportToJson(sampleFullSession);

			// Step 5 has audio
			const stepWithAudio = result.steps.find((s) => s.audioPath !== undefined);
			expect(stepWithAudio).toBeDefined();
			expect(stepWithAudio?.audioPath).toBe("audio/step_5.webm");
		});

		it("should not include audioPath when step has no audio", () => {
			const result = exportToJson(sampleMinimalSession);

			expect(result.steps[0].audioPath).toBeUndefined();
			expect(result.steps[1].audioPath).toBeUndefined();
		});

		it("should generate correct audio paths for annotations with audio", () => {
			const result = exportToJson(sampleFullSession);

			// Third annotation has audio
			expect(result.annotations[2].audioPath).toBe("audio/annotation_3.webm");
		});
	});

	describe("step data preservation", () => {
		it("should preserve element info in exported steps", () => {
			const result = exportToJson(sampleFullSession);
			const clickStep = result.steps.find(
				(s) => s.type === "click" && s.element,
			);

			expect(clickStep?.element).toBeDefined();
			expect(clickStep?.element?.tag).toBe("button");
			expect(clickStep?.element?.text).toBe("Submit Form");
			expect(clickStep?.element?.selector).toBe("button.submit-btn");
		});

		it("should preserve input values in exported steps", () => {
			const result = exportToJson(sampleFullSession);
			const inputStep = result.steps.find(
				(s) => s.type === "input" && !s.isSensitive,
			);

			expect(inputStep?.inputValue).toBe("user@example.com");
			expect(inputStep?.isSensitive).toBe(false);
		});

		it("should preserve sensitive field flags", () => {
			const result = exportToJson(sampleFullSession);
			const sensitiveStep = result.steps.find((s) => s.isSensitive === true);

			expect(sensitiveStep).toBeDefined();
			expect(sensitiveStep?.inputValue).toBe("********");
			expect(sensitiveStep?.isSensitive).toBe(true);
		});

		it("should preserve URL and tabTitle for each step", () => {
			const result = exportToJson(sampleFullSession);

			for (const step of result.steps) {
				expect(step.url).toBeDefined();
				expect(step.tabTitle).toBeDefined();
			}
		});
	});

	describe("annotation data preservation", () => {
		it("should preserve annotation text", () => {
			const result = exportToJson(sampleFullSession);

			expect(result.annotations[0].text).toBe(
				"This is a helpful note about the current step.",
			);
		});

		it("should preserve annotation timestamps", () => {
			const result = exportToJson(sampleFullSession);

			expect(result.annotations[0].timestamp).toBe(7500);
			expect(result.annotations[1].timestamp).toBe(12500);
		});
	});

	describe("edge cases", () => {
		it("should handle sessions with special characters in title", () => {
			const result = exportToJson(sampleSessionWithSpecialChars);

			expect(result.title).toBe("Recording with Special Characters! @#$%^&*()");
		});

		it("should handle session without endTime (still recording)", () => {
			const ongoingSession = {
				...sampleMinimalSession,
				endTime: undefined,
				isRecording: true,
			};
			const result = exportToJson(ongoingSession);

			expect(result.endTime).toBeUndefined();
		});

		it("should not include internal properties like tabId and trackedTabIds", () => {
			const result = exportToJson(sampleFullSession);

			// The exported type shouldn't have these properties
			expect("trackedTabIds" in result).toBe(false);
			expect("isRecording" in result).toBe(false);

			// Steps shouldn't have tabId (only internal)
			for (const step of result.steps) {
				expect("tabId" in step).toBe(false);
			}
		});
	});
});
