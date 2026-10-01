/**
 * Binds the extension's ZIP writer to `shared/recording-export-contract.json`.
 *
 * The htrcli Go writer implements the same format and is pinned to the same
 * contract by htrcli/internal/commands/recordings_export_test.go. Together
 * these two tests make it impossible for either producer to change the bundle
 * layout without the contract — and therefore the other producer — being
 * updated too.
 */

import { describe, expect, it } from "bun:test";
import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { sampleEmptySession, sampleFullSession } from "../test/fixtures";
import { buildRecordingZip } from "./exportZip";

interface ExportContract {
	manifestFile: string;
	readmeFile: string;
	bundleFiles: string[];
	directories: string[];
	compression: string;
	patterns: Record<string, string>;
	paths: Record<string, string>;
}

/**
 * Locate the contract by walking up to the repo root.
 *
 * A hardcoded `../../../shared/...` would silently break if this file moved or
 * if the runner used a different CWD; searching for a file that is known to sit
 * at the repo root is stable against both.
 */
function loadContract(): ExportContract {
	let dir = dirname(fileURLToPath(import.meta.url));
	for (let i = 0; i < 12; i++) {
		const candidate = join(dir, "shared", "recording-export-contract.json");
		try {
			return JSON.parse(readFileSync(candidate, "utf8")) as ExportContract;
		} catch {
			dir = dirname(dir);
			if (dir === resolve(dir, "..")) break;
		}
	}
	throw new Error(
		"shared/recording-export-contract.json not found walking up from " +
			fileURLToPath(import.meta.url),
	);
}

const contract = loadContract();

/** Fill a pattern's single %d with n, mirroring Go's Sprintf. */
function fmt(pattern: string, n: number): string {
	const count = (pattern.match(/%d/g) ?? []).length;
	expect(count).toBe(1); // every contract pattern carries exactly one index
	return pattern.replace("%d", String(n));
}

/** The entry names a full session must produce, derived from the contract. */
function expectedEntries(): string[] {
	const names = new Set<string>(contract.bundleFiles);
	const { paths } = contract;
	// Only media the session actually carries becomes an entry — a step with a
	// screenshot but no audio note must not be expected to yield a .webm.
	sampleFullSession.steps.forEach((step, i) => {
		if (step.screenshotData) names.add(fmt(paths.stepScreenshot, i + 1));
		if (step.audioData) names.add(fmt(paths.stepAudio, i + 1));
	});
	sampleFullSession.annotations.forEach((ann, i) => {
		if (ann.screenshotData) names.add(fmt(paths.annotationScreenshot, i + 1));
		if (ann.audioData) names.add(fmt(paths.annotationAudio, i + 1));
	});
	return [...names].sort();
}

/** Entry names the builder produced, minus JSZip's implicit folder entries. */
async function actualEntryNames(session: typeof sampleFullSession) {
	const zip = await buildRecordingZip(session);
	const all = Object.keys(zip.files);
	// JSZip materialises a folder as a real entry ending in "/". The contract
	// lists directories separately, so they are not treated as files.
	return all.filter((n) => !n.endsWith("/")).sort();
}

describe("recording export bundle contract", () => {
	it("produces exactly the entry names the contract specifies", async () => {
		expect(await actualEntryNames(sampleFullSession)).toEqual(
			expectedEntries(),
		);
	});

	it("numbers media by 1-based position, matching the Go writer", async () => {
		const names = await actualEntryNames(sampleFullSession);
		const stepShot = fmt(contract.paths.stepScreenshot, 1);
		expect(names).toContain(stepShot);
		// No zero-indexed variant may appear — that was a real off-by-one
		// disagreement between the two producers' numbering.
		expect(names).not.toContain(fmt(contract.paths.stepScreenshot, 0));
		// The last step must be present too.
		expect(names).toContain(
			fmt(contract.paths.stepScreenshot, sampleFullSession.steps.length),
		);
	});

	it("keeps only the manifest and README for a session with no media", async () => {
		const names = await actualEntryNames(sampleEmptySession);
		expect(names).toEqual([...contract.bundleFiles].sort());
	});

	it("writes the manifest with path references, never inline base64", async () => {
		const zip = await buildRecordingZip(sampleFullSession);
		const manifest = zip.files[contract.manifestFile];
		expect(manifest).toBeDefined();
		const text = await manifest.async("string");
		expect(text).not.toContain("base64");
		// Every screenshot the session carries must be referenced by path.
		const parsed = JSON.parse(text) as {
			steps: Array<{ screenshotPath?: string }>;
		};
		for (const [i, step] of parsed.steps.entries()) {
			if (sampleFullSession.steps[i].screenshotData) {
				expect(step.screenshotPath).toBe(
					fmt(contract.paths.stepScreenshot, i + 1),
				);
			}
		}
	});

	it("references media paths that the contract agrees with", () => {
		// Guards the contract itself: a pattern and its path must agree, or the
		// two halves of the file could be edited inconsistently.
		const dirs = new Set(contract.directories);
		for (const [patternKey, pathKey] of [
			["stepScreenshot", "stepScreenshot"],
			["stepAudio", "stepAudio"],
			["annotationScreenshot", "annotationScreenshot"],
			["annotationAudio", "annotationAudio"],
		] as const) {
			// Compare the UNFORMATTED pattern: `paths` still contains the %d
			// placeholder, so substituting here would compare "1" against "%d".
			const name = contract.patterns[patternKey];
			const dir = contract.paths[pathKey].split("/")[0];
			expect(dirs.has(dir)).toBe(true);
			expect(contract.paths[pathKey]).toBe(`${dir}/${name}`);
		}
	});

	it("uses the compression method the contract names", () => {
		// The Go writer must match: a Store-only zip still unzips everywhere,
		// so only an explicit assertion catches the divergence.
		expect(contract.compression).toBe("deflate");
	});
});
