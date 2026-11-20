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

	test("forwards the protocol timeout for synchronous loops", async () => {
		const calls: Record<string, unknown>[] = [];
		const send: CdpSend = (_method, params) => {
			calls.push(params);
			return Promise.resolve({ result: { type: "number", value: 1 } });
		};

		await cdpEvaluate(send, "1", 30000);

		expect(calls[0].timeout).toBe(30000);
		expect(calls[0].awaitPromise).toBe(true);
	});

	test("rejects when the send promise never settles", async () => {
		// The protocol timeout does not cover `awaitPromise`, so a never-settling
		// send must be bounded in JS or it holds the debugger attach forever.
		const send: CdpSend = () => new Promise(() => {});
		await expect(
			cdpEvaluate(send, "await new Promise(() => {})", 20),
		).rejects.toThrow(/timed out after 20ms/);
	});

	test("omits the timeout when none is asked for", async () => {
		const calls: Record<string, unknown>[] = [];
		const send: CdpSend = (_method, params) => {
			calls.push(params);
			return Promise.resolve({ result: { type: "number", value: 1 } });
		};

		await cdpEvaluate(send, "1");

		expect("timeout" in calls[0]).toBe(false);
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
