import { describe, expect, test } from "bun:test";
import type { ControlStatusSuppressMessage } from "../types/recording";
import {
	setControlStatusSuppressedInTab,
	withControlStatusSuppressed,
} from "./captureSuppression";

function recorder() {
	const calls: Array<{ tabId: number; message: ControlStatusSuppressMessage }> =
		[];
	const send = async (tabId: number, message: ControlStatusSuppressMessage) => {
		calls.push({ tabId, message });
	};
	return { calls, send };
}

describe("captureSuppression", () => {
	test("setControlStatusSuppressedInTab sends the typed message", async () => {
		const { calls, send } = recorder();
		await setControlStatusSuppressedInTab(42, true, send);
		expect(calls).toEqual([
			{
				tabId: 42,
				message: { type: "CONTROL_STATUS_SUPPRESS", suppressed: true },
			},
		]);
	});

	test("hides before capture and restores after", async () => {
		const { calls, send } = recorder();
		const order: string[] = [];
		const result = await withControlStatusSuppressed(
			7,
			async () => {
				order.push("capture");
				return "png";
			},
			send,
		);

		expect(result).toBe("png");
		expect(order).toEqual(["capture"]);
		expect(calls.map((c) => c.message.suppressed)).toEqual([true, false]);
		expect(calls.every((c) => c.tabId === 7)).toBe(true);
	});

	test("restores even when the capture throws", async () => {
		const { calls, send } = recorder();
		await expect(
			withControlStatusSuppressed(
				7,
				async () => {
					throw new Error("capture failed");
				},
				send,
			),
		).rejects.toThrow("capture failed");
		expect(calls.map((c) => c.message.suppressed)).toEqual([true, false]);
	});

	test("a tab without a content script does not block capture", async () => {
		const calls: ControlStatusSuppressMessage[] = [];
		const send = async (
			_tabId: number,
			message: ControlStatusSuppressMessage,
		) => {
			calls.push(message);
			throw new Error("Could not establish connection");
		};
		const result = await withControlStatusSuppressed(9, async () => "ok", send);

		expect(result).toBe("ok");
		expect(calls.map((m) => m.suppressed)).toEqual([true, false]);
	});
});
