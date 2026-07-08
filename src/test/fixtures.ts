import type {
	Annotation,
	ElementInfo,
	RecordingSession,
	RecordingStep,
} from "../types/recording";

/**
 * Test fixtures for HTR NControl tests
 */

// Sample element info
export const sampleButtonElement: ElementInfo = {
	tag: "button",
	text: "Submit Form",
	selector: "button.submit-btn",
	id: "submit-btn",
	className: "submit-btn primary",
	ariaLabel: "Submit the form",
};

export const sampleInputElement: ElementInfo = {
	tag: "input",
	text: "Email Address",
	selector: "input#email",
	type: "email",
	name: "email",
	id: "email",
	className: "form-control",
};

export const samplePasswordElement: ElementInfo = {
	tag: "input",
	text: "Password",
	selector: "input#password",
	type: "password",
	name: "password",
	id: "password",
	className: "form-control",
};

export const sampleLinkElement: ElementInfo = {
	tag: "a",
	text: "Learn More About Our Services",
	selector: "a.learn-more",
	className: "learn-more link",
};

// Sample recording steps
export const sampleNavigationStep: RecordingStep = {
	id: "step_1234567890_abc123",
	timestamp: 0,
	type: "navigation",
	tabId: 1,
	tabTitle: "Example Website - Home",
	url: "https://example.com/",
	screenshotData:
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
};

export const sampleClickStep: RecordingStep = {
	id: "step_1234567891_def456",
	timestamp: 5000,
	type: "click",
	tabId: 1,
	tabTitle: "Example Website - Home",
	url: "https://example.com/",
	element: sampleButtonElement,
	screenshotData:
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
};

export const sampleInputStep: RecordingStep = {
	id: "step_1234567892_ghi789",
	timestamp: 10000,
	type: "input",
	tabId: 1,
	tabTitle: "Example Website - Login",
	url: "https://example.com/login",
	element: sampleInputElement,
	inputValue: "user@example.com",
	isSensitive: false,
	screenshotData:
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
};

export const sampleSensitiveInputStep: RecordingStep = {
	id: "step_1234567893_jkl012",
	timestamp: 15000,
	type: "input",
	tabId: 1,
	tabTitle: "Example Website - Login",
	url: "https://example.com/login",
	element: samplePasswordElement,
	inputValue: "********",
	isSensitive: true,
	screenshotData:
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
};

export const sampleStepWithAudio: RecordingStep = {
	id: "step_1234567894_mno345",
	timestamp: 20000,
	type: "click",
	tabId: 1,
	tabTitle: "Example Website - Dashboard",
	url: "https://example.com/dashboard",
	element: sampleLinkElement,
	screenshotData:
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
	audioData:
		"data:audio/webm;base64,GkXfo59ChoEBQveBAULygQRC84EIQoKEd2VibUKHgQJChYECGFOAZwH/",
};

// Sample annotations
export const sampleAnnotation: Annotation = {
	id: "ann_1234567890_xyz123",
	timestamp: 7500,
	text: "This is a helpful note about the current step.",
};

export const sampleAnnotationWithScreenshot: Annotation = {
	id: "ann_1234567891_uvw456",
	timestamp: 12500,
	text: "Pay attention to this important form field.",
	screenshotData:
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
};

export const sampleAnnotationWithAudio: Annotation = {
	id: "ann_1234567892_rst789",
	timestamp: 17500,
	text: "Listen to additional instructions.",
	audioData:
		"data:audio/webm;base64,GkXfo59ChoEBQveBAULygQRC84EIQoKEd2VibUKHgQJChYECGFOAZwH/",
};

// Sample recording sessions
export const sampleEmptySession: RecordingSession = {
	id: "session_empty_123",
	title: "Empty Recording",
	startTime: 1700000000000,
	endTime: 1700000060000,
	isRecording: false,
	hasAudio: false,
	steps: [],
	annotations: [],
	trackedTabIds: [1],
};

export const sampleMinimalSession: RecordingSession = {
	id: "session_minimal_456",
	title: "Minimal Recording",
	startTime: 1700000000000,
	endTime: 1700000030000,
	isRecording: false,
	hasAudio: false,
	steps: [sampleNavigationStep, sampleClickStep],
	annotations: [],
	trackedTabIds: [1],
};

export const sampleFullSession: RecordingSession = {
	id: "session_full_789",
	title: "Complete How-To Guide: Login Process",
	startTime: 1700000000000,
	endTime: 1700000120000,
	isRecording: false,
	hasAudio: true,
	steps: [
		sampleNavigationStep,
		sampleClickStep,
		sampleInputStep,
		sampleSensitiveInputStep,
		sampleStepWithAudio,
	],
	annotations: [
		sampleAnnotation,
		sampleAnnotationWithScreenshot,
		sampleAnnotationWithAudio,
	],
	trackedTabIds: [1],
};

export const sampleSessionWithSpecialChars: RecordingSession = {
	id: "session_special_012",
	title: "Recording with Special Characters! @#$%^&*()",
	startTime: 1700000000000,
	endTime: 1700000060000,
	isRecording: false,
	hasAudio: false,
	steps: [sampleNavigationStep],
	annotations: [],
	trackedTabIds: [1],
};

// Helper function to create a step with custom properties
export function createStep(
	overrides: Partial<RecordingStep> = {},
): RecordingStep {
	return {
		id: `step_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
		timestamp: 0,
		type: "click",
		tabId: 1,
		tabTitle: "Test Page",
		url: "https://test.com/",
		...overrides,
	};
}

// Helper function to create an annotation with custom properties
export function createAnnotation(
	overrides: Partial<Annotation> = {},
): Annotation {
	return {
		id: `ann_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
		timestamp: 0,
		text: "Test annotation",
		...overrides,
	};
}

// Helper function to create a session with custom properties
export function createSession(
	overrides: Partial<RecordingSession> = {},
): RecordingSession {
	return {
		id: `session_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
		title: "Test Recording",
		startTime: Date.now(),
		isRecording: false,
		hasAudio: false,
		steps: [],
		annotations: [],
		trackedTabIds: [1],
		...overrides,
	};
}
