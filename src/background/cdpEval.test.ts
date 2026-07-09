import { describe, expect, test } from "bun:test";
import { type CdpSend, cdpEvaluate } from "./cdpEval";

// Builds a stubbed Runtime.evaluate sender that records the expressions it
// receives and replies from a scripted queue of results.
function stubSend(results: unknown[]): {
	send: CdpSend;
	expressions: string[];
} {
	const expressions: string[] = [];
	const queue = [...results];
	const send: CdpSend = (method, params) => {
		expect(method).toBe("Runtime.evaluate");
		expressions.push(params.expression as string);
		return Promise.resolve(queue.shift());
	};
	return { send, expressions };
}

const syntaxError = {
	result: { type: "object", value: undefined },
	exceptionDetails: {
		text: "Uncaught",
		exception: {
			className: "SyntaxError",
			description: "SyntaxError: Illegal return statement",
		},
	},
};

describe("cdpEvaluate", () => {
	test("plain expression evaluates once and returns its value", async () => {
		const { send, expressions } = stubSend([
			{ result: { type: "string", value: "Example Domain" } },
		]);
		const value = await cdpEvaluate(send, "document.title");
		expect(value).toBe("Example Domain");
		expect(expressions).toEqual(["document.title"]);
	});

	test("multi-statement script retries as async function body", async () => {
		const { send, expressions } = stubSend([
			syntaxError,
			{ result: { type: "number", value: 42 } },
		]);
		const value = await cdpEvaluate(send, "const n = 21; return n * 2;");
		expect(value).toBe(42);
		expect(expressions[1]).toBe(
			"(async () => { const n = 21; return n * 2; })()",
		);
	});

	test("script that throws surfaces its own error message", async () => {
		const { send } = stubSend([
			{
				result: { type: "object", value: undefined },
				exceptionDetails: {
					text: "Uncaught",
					exception: {
						className: "Error",
						description: "Error: boom",
					},
				},
			},
		]);
		expect(cdpEvaluate(send, "throw new Error('boom')")).rejects.toThrow(
			"Error: boom",
		);
	});

	test("script invalid in both modes reports the SyntaxError", async () => {
		const { send, expressions } = stubSend([syntaxError, syntaxError]);
		expect(cdpEvaluate(send, "return }{")).rejects.toThrow(
			"Illegal return statement",
		);
		// Allow the rejection to settle before asserting call count.
		await Promise.resolve();
		expect(expressions.length).toBe(2);
	});
});
