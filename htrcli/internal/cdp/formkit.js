// Form helpers for htrcli `pick` and `fill-form` over CDP.
//
// This file is ONE object-literal expression. Go embeds it and calls a method
// via runScoped, which compiles it inside the target realm (top document or
// the --frame iframe), so every global here binds to that realm. Elements are
// addressed by a CSS path produced by the DOM bundle's findAll (`selector`
// field), which is how htrcli selector syntax (name=, label=, text= …) is
// bridged to plain querySelector. Methods throw on failure; runScoped turns a
// throw into a Go error.
({
	el(path) {
		const e = document.querySelector(path);
		if (!e) throw new Error(`element ${path} is no longer in the document`);
		return e;
	},

	norm(s) {
		return (s || "").replace(/\s+/g, " ").trim().toLowerCase();
	},

	text(e) {
		return (e.textContent || "").replace(/\s+/g, " ").trim();
	},

	visible(e) {
		const r = e.getBoundingClientRect();
		if (r.width === 0 || r.height === 0) return false;
		const cs = window.getComputedStyle(e);
		return cs.display !== "none" && cs.visibility !== "hidden";
	},

	// The element that receives focus/keys: the matched <input>, an <input>
	// inside a matched container, or a role=combobox element itself.
	comboInput(path) {
		const e = this.el(path);
		if (e.tagName === "INPUT") return e;
		const inner =
			e.querySelector('input[role="combobox"]') || e.querySelector("input");
		if (inner) return inner;
		if (e.getAttribute("role") === "combobox") return e;
		throw new Error(
			`${path} is not a combobox: it is not an <input>, has no <input> inside and no role=combobox`,
		);
	},

	// Listbox named by aria-controls / aria-owns on the input or its nearest
	// role=combobox ancestor. Re-read on every call: many widgets only set
	// these attributes once the menu is open.
	comboListbox(input) {
		const hosts = [input, input.closest('[role="combobox"]')];
		for (const h of hosts) {
			if (!h) continue;
			for (const attr of ["aria-controls", "aria-owns"]) {
				for (const id of (h.getAttribute(attr) || "").split(/\s+/)) {
					if (!id) continue;
					const lb = document.getElementById(id);
					if (lb) return lb;
				}
			}
		}
		return null;
	},

	// Options of THIS combobox only. Without an aria-controls/aria-owns link the
	// widget's own container is searched; if that finds no listbox, the error
	// says so rather than reading options from the whole document.
	comboOptionEls(path) {
		const input = this.comboInput(path);
		const lb = this.comboListbox(input) || this.comboContainerListbox(input);
		if (!lb) {
			throw new Error(
				`${path}: no listbox is associated with this combobox (no aria-controls/aria-owns, and no listbox or menu in its container); not guessing its options`,
			);
		}
		return Array.from(lb.querySelectorAll('[role="option"]')).filter((o) =>
			this.visible(o),
		);
	},

	// The nearest ancestor of the input that holds an OPEN (visible) listbox or
	// menu; closed menus that widgets keep in the DOM are ignored. Two open ones
	// in that container are ambiguous, so that is an error, not a guess.
	comboContainerListbox(input) {
		for (let c = input.parentElement; c; c = c.parentElement) {
			const found = Array.from(
				c.querySelectorAll('[role="listbox"], [role="menu"]'),
			).filter((l) => this.visible(l));
			if (found.length === 1) return found[0];
			if (found.length > 1) {
				throw new Error(
					`${this.describe(input)}: ${found.length} open listboxes or menus share its container; cannot tell which one belongs to this combobox`,
				);
			}
		}
		return null;
	},

	// Scroll into view, clear any stale query text, and focus — without the
	// bundle's actionable-wait, which rejects react-select's 2px input as
	// "not visible".
	comboPrepare(path) {
		const input = this.comboInput(path);
		// "instant": pages with html{scroll-behavior:smooth} would otherwise
		// animate, and coordinates read afterwards would be stale.
		input.scrollIntoView({
			block: "center",
			inline: "center",
			behavior: "instant",
		});
		if (input.tagName === "INPUT" && input.value !== "") {
			const setter = Object.getOwnPropertyDescriptor(
				window.HTMLInputElement.prototype,
				"value",
			).set;
			setter.call(input, "");
			input.dispatchEvent(new Event("input", { bubbles: true }));
		}
		input.focus();
		return { focused: document.activeElement === input };
	},

	comboOptions(path) {
		return this.comboOptionEls(path).map((o) => this.text(o));
	},

	// Viewport centre of the single visible option whose normalised text is
	// exactly `want` (re-found by text, not index: the list may re-render
	// between the poll that chose it and this call).
	optionPoint(path, want) {
		const w = this.norm(want);
		const matches = this.comboOptionEls(path).filter(
			(o) => this.norm(o.textContent) === w,
		);
		if (matches.length !== 1) {
			throw new Error(
				`option ${JSON.stringify(want)} is no longer uniquely present (${matches.length} matches)`,
			);
		}
		const o = matches[0];
		o.scrollIntoView({
			block: "nearest",
			inline: "nearest",
			behavior: "instant",
		});
		const r = o.getBoundingClientRect();
		const x = r.left + r.width / 2;
		const y = r.top + r.height / 2;
		if (x < 0 || y < 0 || x >= window.innerWidth || y >= window.innerHeight) {
			throw new Error(
				`option ${JSON.stringify(want)} is outside the viewport at (${Math.round(x)}, ${Math.round(y)}) after scrolling; a trusted click there would miss`,
			);
		}
		return { x, y };
	},

	// Short description of an element for error messages.
	describe(e) {
		if (e.id) return `${e.tagName.toLowerCase()}#${e.id}`;
		const cls =
			(typeof e.className === "string"
				? e.className.trim().split(/\s+/)[0]
				: "") || "";
		return e.tagName.toLowerCase() + (cls ? `.${cls}` : "");
	},

	// What the widget displays as selected, plus where it was read from.
	// react-select: the .select__single-value (or multi-value labels) inside
	// the input's OWN .select__value-container (else its control). When such a
	// container exists, the query text in the input is never used: after a
	// missed click it still holds the typed text, and reading it would report a
	// selection that never happened. Plain autocompletes (no container) use the
	// input's value, then aria-valuetext.
	comboValue(path) {
		const input = this.comboInput(path);
		const scope =
			input.closest('.select__value-container, [class*="__value-container"]') ||
			input.closest('.select__control, [class*="__control"]');
		if (scope) {
			const sv = scope.querySelector(
				'.select__single-value, [class*="singleValue"], [class*="single-value"]',
			);
			if (sv)
				return {
					text: this.text(sv),
					from: `${this.describe(sv)} in ${this.describe(scope)}`,
				};
			const mv = Array.from(
				scope.querySelectorAll(
					'.select__multi-value__label, [class*="multi-value__label"]',
				),
			);
			if (mv.length)
				return {
					text: mv.map((m) => this.text(m)).join(", "),
					from: `multi-value labels in ${this.describe(scope)}`,
				};
			return {
				text: "",
				from: `no selected-value element in ${this.describe(scope)}`,
			};
		}
		if (input.tagName === "INPUT" && input.value)
			return { text: input.value, from: `value of ${this.describe(input)}` };
		return {
			text: input.getAttribute("aria-valuetext") || "",
			from: `aria-valuetext of ${this.describe(input)}`,
		};
	},

	selectOptions(path) {
		const e = this.el(path);
		if (e.tagName !== "SELECT") throw new Error(`${path} is not a <select>`);
		return Array.from(e.options).map((o) => ({
			value: o.value,
			text: this.text(o),
		}));
	},

	readSelect(path) {
		const e = this.el(path);
		if (e.tagName !== "SELECT") throw new Error(`${path} is not a <select>`);
		const o = e.options[e.selectedIndex];
		return { value: e.value, text: o ? this.text(o) : "" };
	},

	// Current state of a form field for the fill-form read-back pass.
	readField(path, kind) {
		const e = this.el(path);
		switch (kind) {
			case "text":
				return e.value;
			case "select": {
				const o = e.options[e.selectedIndex];
				return { value: e.value, text: o ? this.text(o) : "" };
			}
			case "checkbox":
			case "radio":
				return e.checked;
			case "file":
				return Array.from(e.files || []).map((f) => f.name);
			case "combobox":
				return this.comboValue(path).text;
			default:
				throw new Error(`unknown field kind ${kind}`);
		}
	},
});
