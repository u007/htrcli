/**
 * Sensitive Field Detection
 * Identifies form fields that contain sensitive information
 * Values will be masked as '********' in recordings
 */

// Input types that are inherently sensitive
const SENSITIVE_INPUT_TYPES = ["password"];

// Autocomplete values that indicate sensitive data
const SENSITIVE_AUTOCOMPLETE_VALUES = [
	"current-password",
	"new-password",
	"cc-number",
	"cc-csc",
	"cc-exp",
	"cc-exp-month",
	"cc-exp-year",
	"cc-name",
	"cc-type",
];

// Patterns in name/id attributes that suggest sensitive data
const SENSITIVE_NAME_PATTERNS = [
	/password/i,
	/passwd/i,
	/secret/i,
	/token/i,
	/api[_-]?key/i,
	/ssn/i,
	/social[_-]?security/i,
	/cvv/i,
	/cvc/i,
	/card[_-]?number/i,
	/credit[_-]?card/i,
	/cc[_-]?num/i,
	/account[_-]?number/i,
	/routing[_-]?number/i,
	/pin/i,
	/security[_-]?code/i,
	/otp/i,
	/two[_-]?factor/i,
	/2fa/i,
	/auth[_-]?code/i,
	/verification[_-]?code/i,
];

// Patterns in placeholder/label that suggest sensitive data
const SENSITIVE_PLACEHOLDER_PATTERNS = [
	/password/i,
	/secret/i,
	/ssn/i,
	/social security/i,
	/credit card/i,
	/card number/i,
	/cvv/i,
	/cvc/i,
	/security code/i,
	/verification code/i,
];

/**
 * Check if an input element is a password field
 */
function isPasswordType(element: HTMLInputElement): boolean {
	return SENSITIVE_INPUT_TYPES.includes(element.type.toLowerCase());
}

/**
 * Check if the autocomplete attribute indicates sensitive data
 */
function hasSensitiveAutocomplete(
	element: HTMLInputElement | HTMLTextAreaElement,
): boolean {
	const autocomplete = element.autocomplete?.toLowerCase() || "";
	return SENSITIVE_AUTOCOMPLETE_VALUES.some(
		(value) => autocomplete === value || autocomplete.includes(value),
	);
}

/**
 * Check if the name or id attribute matches sensitive patterns
 */
function hasSensitiveNameOrId(
	element: HTMLInputElement | HTMLTextAreaElement,
): boolean {
	const name = element.name?.toLowerCase() || "";
	const id = element.id?.toLowerCase() || "";

	return SENSITIVE_NAME_PATTERNS.some(
		(pattern) => pattern.test(name) || pattern.test(id),
	);
}

/**
 * Check if the placeholder matches sensitive patterns
 */
function hasSensitivePlaceholder(
	element: HTMLInputElement | HTMLTextAreaElement,
): boolean {
	const placeholder = element.placeholder?.toLowerCase() || "";

	return SENSITIVE_PLACEHOLDER_PATTERNS.some((pattern) =>
		pattern.test(placeholder),
	);
}

/**
 * Check if the associated label matches sensitive patterns
 */
function hasSensitiveLabel(
	element: HTMLInputElement | HTMLTextAreaElement,
): boolean {
	// Check for explicit label via for attribute
	if (element.id) {
		const label = document.querySelector(`label[for="${element.id}"]`);
		if (label) {
			const labelText = label.textContent?.toLowerCase() || "";
			if (
				SENSITIVE_PLACEHOLDER_PATTERNS.some((pattern) =>
					pattern.test(labelText),
				)
			) {
				return true;
			}
		}
	}

	// Check for implicit label (input inside label)
	const parentLabel = element.closest("label");
	if (parentLabel) {
		const labelText = parentLabel.textContent?.toLowerCase() || "";
		if (
			SENSITIVE_PLACEHOLDER_PATTERNS.some((pattern) => pattern.test(labelText))
		) {
			return true;
		}
	}

	// Check aria-label
	const ariaLabel = element.getAttribute("aria-label")?.toLowerCase() || "";
	if (
		SENSITIVE_PLACEHOLDER_PATTERNS.some((pattern) => pattern.test(ariaLabel))
	) {
		return true;
	}

	return false;
}

/**
 * Check if an element has CSS that hides text (like password masking)
 */
function hasTextMaskingStyle(element: HTMLInputElement): boolean {
	const style = window.getComputedStyle(element);
	const webkitTextSecurity = style.getPropertyValue("-webkit-text-security");
	return (
		webkitTextSecurity === "disc" ||
		webkitTextSecurity === "circle" ||
		webkitTextSecurity === "square"
	);
}

/**
 * Main function to determine if a form element contains sensitive data
 */
export function isSensitiveField(element: Element): boolean {
	// Only check input and textarea elements
	if (
		!(element instanceof HTMLInputElement) &&
		!(element instanceof HTMLTextAreaElement)
	) {
		return false;
	}

	// Check if it's a password-type input
	if (element instanceof HTMLInputElement && isPasswordType(element)) {
		return true;
	}

	// Check autocomplete attribute
	if (hasSensitiveAutocomplete(element)) {
		return true;
	}

	// Check name/id attributes
	if (hasSensitiveNameOrId(element)) {
		return true;
	}

	// Check placeholder
	if (hasSensitivePlaceholder(element)) {
		return true;
	}

	// Check associated labels
	if (hasSensitiveLabel(element)) {
		return true;
	}

	// Check for text masking CSS (used by some custom password fields)
	if (element instanceof HTMLInputElement && hasTextMaskingStyle(element)) {
		return true;
	}

	return false;
}

/**
 * Mask a sensitive value
 */
export function maskValue(_value: string): string {
	return "********";
}

/**
 * Get the appropriate value for recording (masked if sensitive)
 */
export function getRecordableValue(
	element: HTMLInputElement | HTMLTextAreaElement,
): {
	value: string;
	isSensitive: boolean;
} {
	const sensitive = isSensitiveField(element);
	return {
		value: sensitive ? maskValue(element.value) : element.value,
		isSensitive: sensitive,
	};
}

/**
 * Get a description of why a field is considered sensitive (for debugging)
 */
export function getSensitiveReason(element: Element): string | null {
	if (
		!(element instanceof HTMLInputElement) &&
		!(element instanceof HTMLTextAreaElement)
	) {
		return null;
	}

	if (element instanceof HTMLInputElement && isPasswordType(element)) {
		return "password input type";
	}

	if (hasSensitiveAutocomplete(element)) {
		return `sensitive autocomplete: ${element.autocomplete}`;
	}

	if (hasSensitiveNameOrId(element)) {
		return `sensitive name/id: ${element.name || element.id}`;
	}

	if (hasSensitivePlaceholder(element)) {
		return `sensitive placeholder: ${element.placeholder}`;
	}

	if (hasSensitiveLabel(element)) {
		return "sensitive label";
	}

	if (element instanceof HTMLInputElement && hasTextMaskingStyle(element)) {
		return "text masking CSS";
	}

	return null;
}
