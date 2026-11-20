// Shared CDP script evaluation used by both the native-host relay
// (`evaluate`/`debuggerEval` in nativeHost.ts) and the WS-path CDP_EVAL
// handler (index.ts).
//
// Runtime.evaluate only accepts an *expression*, so multi-statement scripts
// with `return` ("const n = 21; return n * 2;") throw "Illegal return
// statement". Mirror the two-mode compile the content-script evaluator uses:
// try the script as a plain expression first (preserves "document.title"
// usage); if that fails to parse (SyntaxError), re-evaluate it as an async
// function body — `(async () => { ... })()` — so `return` and `await` work.
// This is compile-time mode selection, not a silent runtime fallback: a
// script whose *execution* throws surfaces its own error unchanged.

type CdpEvalResult = {
	result: { type: string; value: unknown };
	exceptionDetails?: {
		text?: string;
		exception?: { className?: string; description?: string };
	};
};

export type CdpSend = (
	method: string,
	params: Record<string, unknown>,
) => Promise<unknown>;

function exceptionMessage(details: CdpEvalResult["exceptionDetails"]): string {
	return (
		details?.exception?.description ??
		details?.text ??
		`JS exception: ${JSON.stringify(details)}`
	);
}

function isSyntaxError(details: CdpEvalResult["exceptionDetails"]): boolean {
	return details?.exception?.className === "SyntaxError";
}

/**
 * Bound one CDP round trip in JS. `Runtime.evaluate`'s own `timeout` only
 * terminates *synchronous* execution: it does not bound `awaitPromise`, so a
 * script that returns a never-settling promise would otherwise keep the send
 * pending and hold the debugger attach. Racing the send here turns that into a
 * normal rejection, which lets the caller's `finally` detach.
 */
function withTimeout<T>(work: Promise<T>, timeoutMs: number): Promise<T> {
	if (timeoutMs <= 0) {
		return work;
	}
	return new Promise<T>((resolve, reject) => {
		const timer = setTimeout(() => {
			reject(new Error(`cdpEvaluate: eval timed out after ${timeoutMs}ms`));
		}, timeoutMs);
		work.then(
			(value) => {
				clearTimeout(timer);
				resolve(value);
			},
			(error) => {
				// A rejection after the timer fired is a no-op on the settled
				// promise. intentionally not logged: the caller already reported
				// the timeout, and the abandoned send's late rejection is expected.
				clearTimeout(timer);
				reject(error);
			},
		);
	});
}

/**
 * Evaluate `script` via an injected Runtime.evaluate sender. Returns the
 * script's value; throws with the script's own error message on failure.
 * The caller owns debugger attach/detach.
 */
export async function cdpEvaluate(
	send: CdpSend,
	script: string,
	timeoutMs = 0,
): Promise<unknown> {
	// The protocol `timeout` bounds synchronous loops; `withTimeout` bounds the
	// await itself, which the protocol field does not cover.
	const evaluate = (expression: string) =>
		withTimeout(
			send("Runtime.evaluate", {
				expression,
				awaitPromise: true,
				returnByValue: true,
				...(timeoutMs > 0 ? { timeout: timeoutMs } : {}),
			}) as Promise<CdpEvalResult>,
			timeoutMs,
		);

	let res = await evaluate(script);
	if (res.exceptionDetails && isSyntaxError(res.exceptionDetails)) {
		// Not a valid expression — compile as an async function body instead.
		res = await evaluate(`(async () => { ${script} })()`);
	}
	if (res.exceptionDetails) {
		throw new Error(exceptionMessage(res.exceptionDetails));
	}
	return res.result.value;
}
