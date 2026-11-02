/**
 * Synthetic click sequence shared by element clicks, coordinate clicks and the
 * `pick` option click. Pointer events are dispatched immediately before/after
 * their mouse counterparts (pointerdown before mousedown, pointerup before
 * mouseup), which is the order real browsers produce and frameworks expect.
 */
export function dispatchClickSequence(
	element: Element,
	x: number,
	y: number,
	button: string,
	count: number,
): void {
	const eventInit: MouseEventInit = {
		bubbles: true,
		cancelable: true,
		view: window,
		button: button === "right" ? 2 : 0,
		clientX: x,
		clientY: y,
	};

	const pointerButton = button === "right" ? 2 : 0;
	const pointerInit: PointerEventInit = {
		bubbles: true,
		cancelable: true,
		view: window,
		pointerId: 1,
		pointerType: "mouse",
		button: pointerButton,
		buttons: pointerButton,
		clientX: x,
		clientY: y,
	};
	const makePointer = (type: string) =>
		typeof PointerEvent !== "undefined"
			? new PointerEvent(type, pointerInit)
			: new MouseEvent(type, eventInit);

	const singleClickSequence = [
		"pointerover",
		"mouseover",
		"mouseenter",
		"pointerdown",
		"mousedown",
		"pointerup",
		"mouseup",
		"click",
	];

	for (let i = 0; i < count; i++) {
		for (const eventType of singleClickSequence) {
			if (eventType.startsWith("pointer")) {
				element.dispatchEvent(makePointer(eventType));
			} else {
				element.dispatchEvent(new MouseEvent(eventType, eventInit));
			}
		}
	}

	// dblclick fires after both click sequences, matching the DOM Level 3 event order
	if (count === 2) {
		element.dispatchEvent(new MouseEvent("dblclick", eventInit));
	}
}
