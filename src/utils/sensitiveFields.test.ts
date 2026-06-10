import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { Window } from "happy-dom";

describe("sensitiveFields", () => {
	let window: Window;
	let document: Document;

	// We need to dynamically import the module after setting up globals
	let isSensitiveField: (element: Element) => boolean;
	let maskValue: (value: string) => string;
	let getRecordableValue: (element: HTMLInputElement | HTMLTextAreaElement) => {
		value: string;
		isSensitive: boolean;
	};
	let getSensitiveReason: (element: Element) => string | null;

	beforeEach(async () => {
		window = new Window({ url: "https://localhost:8080" });
		document = window.document as unknown as Document;

		// Set up globals that the sensitiveFields module needs
		globalThis.document = document;
		// @ts-expect-error - happy-dom window is compatible for our tests
		globalThis.window = window;
		// @ts-expect-error - happy-dom types are compatible for our tests
		globalThis.HTMLInputElement = window.HTMLInputElement;
		// @ts-expect-error - happy-dom types are compatible for our tests
		globalThis.HTMLTextAreaElement = window.HTMLTextAreaElement;

		// Clear module cache and re-import
		const modulePath = "../utils/sensitiveFields";
		// Use dynamic import to get fresh module with new globals
		const mod = await import(modulePath);
		isSensitiveField = mod.isSensitiveField;
		maskValue = mod.maskValue;
		getRecordableValue = mod.getRecordableValue;
		getSensitiveReason = mod.getSensitiveReason;
	});

	afterEach(() => {
		window.close();
	});

	describe("isSensitiveField", () => {
		describe("password input type", () => {
			it("should detect password input type", () => {
				const input = document.createElement("input");
				input.type = "password";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should not detect text input type", () => {
				const input = document.createElement("input");
				input.type = "text";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(false);
			});

			it("should not detect email input type by itself", () => {
				const input = document.createElement("input");
				input.type = "email";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(false);
			});
		});

		describe("autocomplete attribute", () => {
			it("should detect current-password autocomplete", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.autocomplete = "current-password";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect new-password autocomplete", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.autocomplete = "new-password";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect cc-number autocomplete", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.autocomplete = "cc-number";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect cc-csc autocomplete", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.autocomplete = "cc-csc";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should not detect username autocomplete", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.autocomplete = "username";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(false);
			});
		});

		describe("name/id patterns", () => {
			it("should detect password in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "user_password";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect ssn in id", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.id = "ssn-field";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect social_security in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "social_security_number";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect card_number in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "card_number";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect credit-card in id", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.id = "credit-card";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect cvv in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "cvv";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect cvc in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "cvc";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect api_key in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "api_key";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect secret in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "client_secret";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect token in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "auth_token";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect otp in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "otp";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect 2fa in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "2fa_code";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect verification_code in name", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "verification_code";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should not detect regular name attributes", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.name = "username";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(false);
			});
		});

		describe("placeholder patterns", () => {
			it("should detect password in placeholder", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.placeholder = "Enter your password";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect credit card in placeholder", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.placeholder = "Credit card number";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect CVV in placeholder", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.placeholder = "CVV";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect social security in placeholder", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.placeholder = "Social Security Number";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should not detect normal placeholder text", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.placeholder = "Enter your email";
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(false);
			});
		});

		describe("label detection", () => {
			it("should detect sensitive explicit label", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.id = "field1";

				const label = document.createElement("label");
				label.setAttribute("for", "field1");
				label.textContent = "Password";

				document.body.appendChild(label);
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect sensitive implicit label (input inside label)", () => {
				const label = document.createElement("label");
				label.textContent = "Credit Card Number: ";

				const input = document.createElement("input");
				input.type = "text";

				label.appendChild(input);
				document.body.appendChild(label);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should detect sensitive aria-label", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.setAttribute("aria-label", "Password");
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(true);
			});

			it("should not detect non-sensitive labels", () => {
				const input = document.createElement("input");
				input.type = "text";
				input.id = "email-field";

				const label = document.createElement("label");
				label.setAttribute("for", "email-field");
				label.textContent = "Email Address";

				document.body.appendChild(label);
				document.body.appendChild(input);

				expect(isSensitiveField(input)).toBe(false);
			});
		});

		describe("non-input elements", () => {
			it("should return false for div elements", () => {
				const div = document.createElement("div");
				document.body.appendChild(div);

				expect(isSensitiveField(div)).toBe(false);
			});

			it("should return false for button elements", () => {
				const button = document.createElement("button");
				document.body.appendChild(button);

				expect(isSensitiveField(button)).toBe(false);
			});

			it("should return false for span elements", () => {
				const span = document.createElement("span");
				document.body.appendChild(span);

				expect(isSensitiveField(span)).toBe(false);
			});
		});

		describe("textarea elements", () => {
			it("should detect sensitive textarea by name", () => {
				const textarea = document.createElement("textarea");
				textarea.name = "password_field";
				document.body.appendChild(textarea);

				expect(isSensitiveField(textarea)).toBe(true);
			});

			it("should not detect non-sensitive textarea", () => {
				const textarea = document.createElement("textarea");
				textarea.name = "comments";
				document.body.appendChild(textarea);

				expect(isSensitiveField(textarea)).toBe(false);
			});
		});
	});

	describe("maskValue", () => {
		it("should return masked string regardless of input", () => {
			expect(maskValue("secret123")).toBe("********");
		});

		it("should return same mask for different lengths", () => {
			expect(maskValue("a")).toBe("********");
			expect(maskValue("a very long password")).toBe("********");
		});

		it("should return mask for empty string", () => {
			expect(maskValue("")).toBe("********");
		});
	});

	describe("getRecordableValue", () => {
		it("should return masked value for sensitive fields", () => {
			const input = document.createElement("input");
			input.type = "password";
			input.value = "mysecretpassword";
			document.body.appendChild(input);

			const result = getRecordableValue(input);

			expect(result.value).toBe("********");
			expect(result.isSensitive).toBe(true);
		});

		it("should return actual value for non-sensitive fields", () => {
			const input = document.createElement("input");
			input.type = "text";
			input.name = "username";
			input.value = "john_doe";
			document.body.appendChild(input);

			const result = getRecordableValue(input);

			expect(result.value).toBe("john_doe");
			expect(result.isSensitive).toBe(false);
		});

		it("should work with textarea elements", () => {
			const textarea = document.createElement("textarea");
			textarea.name = "api_key";
			textarea.value = "sk-abc123xyz";
			document.body.appendChild(textarea);

			const result = getRecordableValue(textarea);

			expect(result.value).toBe("********");
			expect(result.isSensitive).toBe(true);
		});
	});

	describe("getSensitiveReason", () => {
		it("should return reason for password input type", () => {
			const input = document.createElement("input");
			input.type = "password";
			document.body.appendChild(input);

			expect(getSensitiveReason(input)).toBe("password input type");
		});

		it("should return reason for sensitive autocomplete", () => {
			const input = document.createElement("input");
			input.type = "text";
			input.autocomplete = "cc-number";
			document.body.appendChild(input);

			expect(getSensitiveReason(input)).toBe(
				"sensitive autocomplete: cc-number",
			);
		});

		it("should return reason for sensitive name", () => {
			const input = document.createElement("input");
			input.type = "text";
			input.name = "credit_card";
			document.body.appendChild(input);

			expect(getSensitiveReason(input)).toBe("sensitive name/id: credit_card");
		});

		it("should return reason for sensitive placeholder", () => {
			const input = document.createElement("input");
			input.type = "text";
			input.placeholder = "Enter your password";
			document.body.appendChild(input);

			expect(getSensitiveReason(input)).toBe(
				"sensitive placeholder: Enter your password",
			);
		});

		it("should return null for non-sensitive fields", () => {
			const input = document.createElement("input");
			input.type = "text";
			input.name = "username";
			document.body.appendChild(input);

			expect(getSensitiveReason(input)).toBe(null);
		});

		it("should return null for non-input elements", () => {
			const div = document.createElement("div");
			document.body.appendChild(div);

			expect(getSensitiveReason(div)).toBe(null);
		});
	});
});
