/**
 * Full-page screenshot scroll-stitch geometry.
 *
 * Computes the set of scroll positions needed to capture every tile of a
 * page that is larger (in any axis) than the viewport, and the final canvas
 * dimensions in device pixels.
 */

export interface StitchSegment {
	scrollX: number;
	scrollY: number;
}

export interface StitchPlan {
	canvasWidth: number;
	canvasHeight: number;
	segments: StitchSegment[];
}

/**
 * Given the full content dimensions and the viewport size, produce the
 * scroll-stitch plan: the device-pixel canvas and an ordered list of
 * (scrollX, scrollY) positions to capture.
 *
 * The last column/row is clamped so we never scroll past the maximum
 * scroll offset (content – viewport).
 */
export function computeStitchPlan(
	contentWidth: number,
	contentHeight: number,
	viewportWidth: number,
	viewportHeight: number,
	dpr: number,
): StitchPlan {
	const cols = Math.max(1, Math.ceil(contentWidth / viewportWidth));
	const rows = Math.max(1, Math.ceil(contentHeight / viewportHeight));
	const maxScrollX = Math.max(0, contentWidth - viewportWidth);
	const maxScrollY = Math.max(0, contentHeight - viewportHeight);
	const segments: StitchSegment[] = [];

	for (let r = 0; r < rows; r++) {
		for (let c = 0; c < cols; c++) {
			segments.push({
				scrollX: Math.min(c * viewportWidth, maxScrollX),
				scrollY: Math.min(r * viewportHeight, maxScrollY),
			});
		}
	}

	return {
		canvasWidth: Math.round(contentWidth * dpr),
		canvasHeight: Math.round(contentHeight * dpr),
		segments,
	};
}
