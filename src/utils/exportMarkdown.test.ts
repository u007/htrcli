import { describe, expect, it } from "bun:test";
import {
	createSession,
	createStep,
	sampleButtonElement,
	sampleEmptySession,
	sampleFullSession,
	sampleInputElement,
	sampleMinimalSession,
	sampleSessionWithSpecialChars,
} from "../test/fixtures";
import { generateMarkdown } from "../utils/exportMarkdown";

describe("generateMarkdown", () => {
	describe("basic structure", () => {
		it("should generate markdown with title", () => {
			const result = generateMarkdown(sampleMinimalSession);

			expect(result).toContain("# Minimal Recording");
		});

		it("should include recorded date", () => {
			const result = generateMarkdown(sampleMinimalSession);

			expect(result).toContain("_Recorded on");
		});

		it("should include horizontal rules between sections", () => {
			const result = generateMarkdown(sampleMinimalSession);

			expect(result).toContain("---");
		});

		it("should handle empty session", () => {
			const result = generateMarkdown(sampleEmptySession);

			expect(result).toContain("# Empty Recording");
			// Should not contain any step headers
			expect(result).not.toContain("## Step");
		});
	});

	describe("step formatting", () => {
		it("should format navigation steps", () => {
			const result = generateMarkdown(sampleMinimalSession);

			expect(result).toContain("## Step 1:");
			expect(result).toContain("Navigated to page");
		});

		it("should format click steps with element text", () => {
			const result = generateMarkdown(sampleMinimalSession);

			expect(result).toContain('Clicked "Submit Form"');
		});

		it("should format input steps with value", () => {
			const result = generateMarkdown(sampleFullSession);

			expect(result).toContain('Entered "user@example.com"');
		});

		it("should mask sensitive input values", () => {
			const result = generateMarkdown(sampleFullSession);

			expect(result).toContain("Entered sensitive data");
			expect(result).toContain("\\*\\*\\*\\*\\*\\*\\*\\* (sensitive)");
		});

		it("should include timestamps in MM:SS format", () => {
			const result = generateMarkdown(sampleFullSession);

			// 0ms = 00:00
			expect(result).toContain("(00:00)");
			// 5000ms = 00:05
			expect(result).toContain("(00:05)");
		});

		it("should include URL for each step", () => {
			const result = generateMarkdown(sampleMinimalSession);

			expect(result).toContain("**URL:** https://example.com/");
		});

		it("should include element details for clicks", () => {
			const result = generateMarkdown(sampleMinimalSession);

			expect(result).toContain("**Element:** `<button>`");
		});

		it("should hide selector in collapsible details", () => {
			const result = generateMarkdown(sampleMinimalSession);

			expect(result).toContain("<details>");
			expect(result).toContain("<summary>Technical Details</summary>");
			expect(result).toContain("**Selector:** `button.submit-btn`");
			expect(result).toContain("</details>");
		});

		it("should include element ID when present", () => {
			const result = generateMarkdown(sampleMinimalSession);

			expect(result).toContain("**ID:** `submit-btn`");
		});

		it("should include element classes when present", () => {
			const result = generateMarkdown(sampleMinimalSession);

			expect(result).toContain("**Classes:** `submit-btn primary`");
		});
	});

	describe("screenshot handling", () => {
		describe("with embedMedia: false (default)", () => {
			it("should reference external screenshot files", () => {
				const result = generateMarkdown(sampleMinimalSession);

				expect(result).toContain("![Screenshot](./screenshots/step_1.png)");
				expect(result).toContain("![Screenshot](./screenshots/step_2.png)");
			});

			it("should not embed base64 data", () => {
				const result = generateMarkdown(sampleMinimalSession);

				expect(result).not.toContain("data:image/png;base64");
			});
		});

		describe("with embedMedia: true", () => {
			it("should embed screenshots as base64 data URLs", () => {
				const result = generateMarkdown(sampleMinimalSession, {
					embedMedia: true,
				});

				expect(result).toContain("![Screenshot](data:image/png;base64,");
			});

			it("should not reference external files", () => {
				const result = generateMarkdown(sampleMinimalSession, {
					embedMedia: true,
				});

				expect(result).not.toContain("./screenshots/");
			});
		});
	});

	describe("audio handling", () => {
		describe("with embedMedia: false (default)", () => {
			it("should reference external audio files", () => {
				const result = generateMarkdown(sampleFullSession);

				expect(result).toContain("[Listen to audio](./audio/step_5.webm)");
			});
		});

		describe("with embedMedia: true", () => {
			it("should embed audio as HTML audio element", () => {
				const result = generateMarkdown(sampleFullSession, {
					embedMedia: true,
				});

				expect(result).toContain(
					'<audio controls src="data:audio/webm;base64,',
				);
				expect(result).toContain("</audio>");
			});

			it("should include fallback text for audio element", () => {
				const result = generateMarkdown(sampleFullSession, {
					embedMedia: true,
				});

				expect(result).toContain(
					"Your browser does not support the audio element.",
				);
			});
		});
	});

	describe("annotation formatting", () => {
		it("should format annotations as notes", () => {
			const result = generateMarkdown(sampleFullSession);

			expect(result).toContain("## Note 1 (");
		});

		it("should include annotation text as blockquote", () => {
			const result = generateMarkdown(sampleFullSession);

			expect(result).toContain(
				"> This is a helpful note about the current step.",
			);
		});

		it("should include annotation timestamp", () => {
			const result = generateMarkdown(sampleFullSession);

			// 7500ms = 00:07
			expect(result).toContain("Note 1 (00:07)");
		});

		it("should handle annotation screenshots", () => {
			const result = generateMarkdown(sampleFullSession, { embedMedia: false });

			expect(result).toContain(
				"![Annotation Screenshot](./screenshots/note_2.png)",
			);
		});

		it("should embed annotation screenshots when embedMedia is true", () => {
			const result = generateMarkdown(sampleFullSession, { embedMedia: true });

			expect(result).toContain(
				"![Annotation Screenshot](data:image/png;base64,",
			);
		});

		it("should handle annotation audio", () => {
			const result = generateMarkdown(sampleFullSession, { embedMedia: false });

			expect(result).toContain("[Listen to audio](./audio/note_3.webm)");
		});
	});

	describe("timeline ordering", () => {
		it("should interleave steps and annotations by timestamp", () => {
			const result = generateMarkdown(sampleFullSession);

			// Get positions of step 1 (0ms), step 2 (5000ms), note 1 (7500ms), step 3 (10000ms)
			const step1Pos = result.indexOf("Step 1:");
			const step2Pos = result.indexOf("Step 2:");
			const note1Pos = result.indexOf("Note 1");
			const step3Pos = result.indexOf("Step 3:");

			expect(step1Pos).toBeLessThan(step2Pos);
			expect(step2Pos).toBeLessThan(note1Pos);
			expect(note1Pos).toBeLessThan(step3Pos);
		});
	});

	describe("edge cases", () => {
		it("should handle special characters in title", () => {
			const result = generateMarkdown(sampleSessionWithSpecialChars);

			expect(result).toContain(
				"# Recording with Special Characters! @#$%^&*()",
			);
		});

		it("should truncate long element text", () => {
			const sessionWithLongText = createSession({
				steps: [
					createStep({
						type: "click",
						element: {
							...sampleButtonElement,
							text: "This is a very long button text that should be truncated because it exceeds fifty characters",
						},
					}),
				],
			});

			const result = generateMarkdown(sessionWithLongText);

			expect(result).toContain("...");
			expect(result).toContain(
				"This is a very long button text that should be tr",
			);
		});

		it("should handle steps without element info", () => {
			const sessionWithNoElement = createSession({
				steps: [
					createStep({
						type: "click",
						element: undefined,
					}),
				],
			});

			const result = generateMarkdown(sessionWithNoElement);

			expect(result).toContain("Clicked element");
		});

		it("should handle click on element with no text", () => {
			const sessionWithNoText = createSession({
				steps: [
					createStep({
						type: "click",
						element: {
							tag: "div",
							text: "",
							selector: "div.container",
						},
					}),
				],
			});

			const result = generateMarkdown(sessionWithNoText);

			expect(result).toContain("Clicked div");
		});

		it("should format time correctly for longer recordings", () => {
			const sessionWithLongDuration = createSession({
				steps: [
					createStep({
						timestamp: 125000, // 2 minutes and 5 seconds
					}),
				],
			});

			const result = generateMarkdown(sessionWithLongDuration);

			expect(result).toContain("(02:05)");
		});

		it("should include name attribute in technical details", () => {
			const sessionWithNameAttr = createSession({
				steps: [
					createStep({
						type: "input",
						element: sampleInputElement,
						inputValue: "test",
					}),
				],
			});

			const result = generateMarkdown(sessionWithNameAttr);

			expect(result).toContain("**Name:** `email`");
		});
	});
});
