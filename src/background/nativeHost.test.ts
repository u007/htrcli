import { describe, expect, it } from "bun:test";
import { isPermanentError } from "./nativeHost";

describe("isPermanentError", () => {
	it("treats a missing/unregistered host as permanent (no reconnect)", () => {
		expect(isPermanentError("Specified native messaging host not found.")).toBe(
			true,
		);
		expect(isPermanentError("No such native application com.x.host")).toBe(
			true,
		);
		expect(isPermanentError("host not installed")).toBe(true);
		expect(
			isPermanentError(
				"Access to the specified native messaging host is forbidden.",
			),
		).toBe(true);
	});

	it("treats 'Native host has exited' as transient so it reconnects", () => {
		// Chrome reports this when the relay quits because the daemon socket is
		// down. Retrying with backoff must recover once the daemon starts.
		expect(isPermanentError("Native host has exited.")).toBe(false);
	});

	it("treats an unknown/relay-death disconnect as transient", () => {
		expect(isPermanentError("unknown")).toBe(false);
		expect(
			isPermanentError(
				"Error when communicating with the native messaging host.",
			),
		).toBe(false);
	});
});
