import { type DBSchema, type IDBPDatabase, openDB } from "idb";
import type {
	Annotation,
	RecordingSession,
	RecordingStep,
	SessionMetadata,
} from "../types/recording";

const DB_NAME = "HowToRecorderDB";
const DB_VERSION = 1;

// Blob storage types
export interface StoredBlob {
	id: string;
	sessionId: string;
	stepId?: string;
	annotationId?: string;
	type: "screenshot" | "audio";
	data: Blob;
}

// IndexedDB schema definition
interface HowToRecorderDBSchema extends DBSchema {
	sessions: {
		key: string;
		value: SessionMetadata;
		indexes: { "by-startTime": number };
	};
	steps: {
		key: string;
		value: RecordingStep & { sessionId: string };
		indexes: { "by-sessionId": string; "by-timestamp": number };
	};
	annotations: {
		key: string;
		value: Annotation & { sessionId: string };
		indexes: { "by-sessionId": string; "by-timestamp": number };
	};
	blobs: {
		key: string;
		value: StoredBlob;
		indexes: { "by-sessionId": string; "by-stepId": string };
	};
}

let dbInstance: IDBPDatabase<HowToRecorderDBSchema> | null = null;

// Initialize the database
export async function initDB(): Promise<IDBPDatabase<HowToRecorderDBSchema>> {
	if (dbInstance) {
		return dbInstance;
	}

	dbInstance = await openDB<HowToRecorderDBSchema>(DB_NAME, DB_VERSION, {
		upgrade(db) {
			// Sessions store
			if (!db.objectStoreNames.contains("sessions")) {
				const sessionStore = db.createObjectStore("sessions", {
					keyPath: "id",
				});
				sessionStore.createIndex("by-startTime", "startTime");
			}

			// Steps store
			if (!db.objectStoreNames.contains("steps")) {
				const stepStore = db.createObjectStore("steps", { keyPath: "id" });
				stepStore.createIndex("by-sessionId", "sessionId");
				stepStore.createIndex("by-timestamp", "timestamp");
			}

			// Annotations store
			if (!db.objectStoreNames.contains("annotations")) {
				const annotationStore = db.createObjectStore("annotations", {
					keyPath: "id",
				});
				annotationStore.createIndex("by-sessionId", "sessionId");
				annotationStore.createIndex("by-timestamp", "timestamp");
			}

			// Blobs store
			if (!db.objectStoreNames.contains("blobs")) {
				const blobStore = db.createObjectStore("blobs", { keyPath: "id" });
				blobStore.createIndex("by-sessionId", "sessionId");
				blobStore.createIndex("by-stepId", "stepId");
			}
		},
	});

	return dbInstance;
}

// Get database instance
export async function getDB(): Promise<IDBPDatabase<HowToRecorderDBSchema>> {
	if (!dbInstance) {
		return initDB();
	}
	return dbInstance;
}

// === Session Operations ===

export async function createSession(session: RecordingSession): Promise<void> {
	const db = await getDB();
	const metadata: SessionMetadata = {
		id: session.id,
		title: session.title,
		startTime: session.startTime,
		endTime: session.endTime,
		hasAudio: session.hasAudio,
		stepCount: session.steps.length,
		annotationCount: session.annotations.length,
	};
	await db.put("sessions", metadata);
}

export async function updateSession(
	session: Partial<RecordingSession> & { id: string },
): Promise<void> {
	const db = await getDB();
	const existing = await db.get("sessions", session.id);
	if (existing) {
		await db.put("sessions", {
			...existing,
			...session,
			stepCount: session.steps?.length ?? existing.stepCount,
			annotationCount: session.annotations?.length ?? existing.annotationCount,
		} as SessionMetadata);
	}
}

export async function getSession(
	sessionId: string,
): Promise<RecordingSession | null> {
	const db = await getDB();
	const metadata = await db.get("sessions", sessionId);
	if (!metadata) return null;

	const steps = await getStepsBySession(sessionId);
	const annotations = await getAnnotationsBySession(sessionId);

	return {
		id: metadata.id,
		title: metadata.title,
		startTime: metadata.startTime,
		endTime: metadata.endTime,
		isRecording: false,
		hasAudio: metadata.hasAudio,
		steps,
		annotations,
		trackedTabIds: [],
	};
}

export async function getAllSessions(): Promise<SessionMetadata[]> {
	const db = await getDB();
	const sessions = await db.getAllFromIndex("sessions", "by-startTime");
	return sessions.reverse(); // Most recent first
}

export async function deleteSession(sessionId: string): Promise<void> {
	const db = await getDB();

	// Delete all related data
	const tx = db.transaction(
		["sessions", "steps", "annotations", "blobs"],
		"readwrite",
	);

	// Delete session
	await tx.objectStore("sessions").delete(sessionId);

	// Delete steps
	const steps = await tx
		.objectStore("steps")
		.index("by-sessionId")
		.getAllKeys(sessionId);
	for (const key of steps) {
		await tx.objectStore("steps").delete(key);
	}

	// Delete annotations
	const annotations = await tx
		.objectStore("annotations")
		.index("by-sessionId")
		.getAllKeys(sessionId);
	for (const key of annotations) {
		await tx.objectStore("annotations").delete(key);
	}

	// Delete blobs
	const blobs = await tx
		.objectStore("blobs")
		.index("by-sessionId")
		.getAllKeys(sessionId);
	for (const key of blobs) {
		await tx.objectStore("blobs").delete(key);
	}

	await tx.done;
}

// === Step Operations ===

export async function addStep(
	sessionId: string,
	step: RecordingStep,
): Promise<void> {
	const db = await getDB();
	await db.put("steps", { ...step, sessionId });

	// Update session step count
	const session = await db.get("sessions", sessionId);
	if (session) {
		session.stepCount++;
		await db.put("sessions", session);
	}
}

export async function getStepsBySession(
	sessionId: string,
): Promise<RecordingStep[]> {
	const db = await getDB();
	const steps = await db.getAllFromIndex("steps", "by-sessionId", sessionId);
	return steps.sort((a, b) => a.timestamp - b.timestamp);
}

export async function updateStep(
	step: RecordingStep & { sessionId: string },
): Promise<void> {
	const db = await getDB();
	await db.put("steps", step);
}

export async function deleteStep(stepId: string): Promise<void> {
	const db = await getDB();
	const step = await db.get("steps", stepId);
	if (step) {
		await db.delete("steps", stepId);

		// Update session step count
		const session = await db.get("sessions", step.sessionId);
		if (session) {
			session.stepCount--;
			await db.put("sessions", session);
		}

		// Delete associated blobs
		const blobs = await db.getAllFromIndex("blobs", "by-stepId", stepId);
		for (const blob of blobs) {
			await db.delete("blobs", blob.id);
		}
	}
}

// === Annotation Operations ===

export async function addAnnotation(
	sessionId: string,
	annotation: Annotation,
): Promise<void> {
	const db = await getDB();
	await db.put("annotations", { ...annotation, sessionId });

	// Update session annotation count
	const session = await db.get("sessions", sessionId);
	if (session) {
		session.annotationCount++;
		await db.put("sessions", session);
	}
}

export async function getAnnotationsBySession(
	sessionId: string,
): Promise<Annotation[]> {
	const db = await getDB();
	const annotations = await db.getAllFromIndex(
		"annotations",
		"by-sessionId",
		sessionId,
	);
	return annotations.sort((a, b) => a.timestamp - b.timestamp);
}

export async function updateAnnotation(
	annotation: Annotation & { sessionId: string },
): Promise<void> {
	const db = await getDB();
	await db.put("annotations", annotation);
}

export async function deleteAnnotation(annotationId: string): Promise<void> {
	const db = await getDB();
	const annotation = await db.get("annotations", annotationId);
	if (annotation) {
		await db.delete("annotations", annotationId);

		// Update session annotation count
		const session = await db.get("sessions", annotation.sessionId);
		if (session) {
			session.annotationCount--;
			await db.put("sessions", session);
		}
	}
}

// === Blob Operations ===

export async function saveBlob(blob: StoredBlob): Promise<void> {
	const db = await getDB();
	await db.put("blobs", blob);
}

export async function getBlob(blobId: string): Promise<StoredBlob | undefined> {
	const db = await getDB();
	return db.get("blobs", blobId);
}

export async function getBlobsBySession(
	sessionId: string,
): Promise<StoredBlob[]> {
	const db = await getDB();
	return db.getAllFromIndex("blobs", "by-sessionId", sessionId);
}

export async function getBlobsByStep(stepId: string): Promise<StoredBlob[]> {
	const db = await getDB();
	return db.getAllFromIndex("blobs", "by-stepId", stepId);
}

// === Utility Functions ===

// Convert base64 to Blob
export function base64ToBlob(base64: string, mimeType: string): Blob {
	const byteCharacters = atob(base64.split(",")[1] || base64);
	const byteNumbers = new Array(byteCharacters.length);
	for (let i = 0; i < byteCharacters.length; i++) {
		byteNumbers[i] = byteCharacters.charCodeAt(i);
	}
	const byteArray = new Uint8Array(byteNumbers);
	return new Blob([byteArray], { type: mimeType });
}

// Convert Blob to base64
export async function blobToBase64(blob: Blob): Promise<string> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onloadend = () => resolve(reader.result as string);
		reader.onerror = reject;
		reader.readAsDataURL(blob);
	});
}

// Export full session with all data
export async function exportSessionData(
	sessionId: string,
): Promise<RecordingSession | null> {
	const session = await getSession(sessionId);
	if (!session) return null;

	// Populate screenshots and audio from blobs
	const blobs = await getBlobsBySession(sessionId);

	for (const step of session.steps) {
		const stepBlobs = blobs.filter((b) => b.stepId === step.id);
		for (const blob of stepBlobs) {
			if (blob.type === "screenshot") {
				step.screenshotData = await blobToBase64(blob.data);
			} else if (blob.type === "audio") {
				step.audioData = await blobToBase64(blob.data);
			}
		}
	}

	return session;
}

// Clear all data (for debugging/reset)
export async function clearAllData(): Promise<void> {
	const db = await getDB();
	await db.clear("sessions");
	await db.clear("steps");
	await db.clear("annotations");
	await db.clear("blobs");
}

// Get storage usage estimate
export async function getStorageUsage(): Promise<{
	used: number;
	quota: number;
}> {
	if (navigator.storage?.estimate) {
		const estimate = await navigator.storage.estimate();
		return {
			used: estimate.usage || 0,
			quota: estimate.quota || 0,
		};
	}
	return { used: 0, quota: 0 };
}
