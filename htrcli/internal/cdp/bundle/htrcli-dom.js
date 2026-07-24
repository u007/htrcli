(function() {
  "use strict";
  const AIA_API_KEY = "50efbade-11e8-4169-abc3-e84e1b4c561b";
  const NAMED_KEYS = {
    Enter: { key: "Enter", code: "Enter", windowsVirtualKeyCode: 13, text: "\r" },
    Tab: { key: "Tab", code: "Tab", windowsVirtualKeyCode: 9, text: "	" },
    Escape: { key: "Escape", code: "Escape", windowsVirtualKeyCode: 27 },
    Backspace: { key: "Backspace", code: "Backspace", windowsVirtualKeyCode: 8 },
    Delete: { key: "Delete", code: "Delete", windowsVirtualKeyCode: 46 },
    ArrowUp: { key: "ArrowUp", code: "ArrowUp", windowsVirtualKeyCode: 38 },
    ArrowDown: { key: "ArrowDown", code: "ArrowDown", windowsVirtualKeyCode: 40 },
    ArrowLeft: { key: "ArrowLeft", code: "ArrowLeft", windowsVirtualKeyCode: 37 },
    ArrowRight: {
      key: "ArrowRight",
      code: "ArrowRight",
      windowsVirtualKeyCode: 39
    },
    Home: { key: "Home", code: "Home", windowsVirtualKeyCode: 36 },
    End: { key: "End", code: "End", windowsVirtualKeyCode: 35 },
    PageUp: { key: "PageUp", code: "PageUp", windowsVirtualKeyCode: 33 },
    PageDown: { key: "PageDown", code: "PageDown", windowsVirtualKeyCode: 34 }
  };
  const SYMBOL_KEYS = {
    " ": { code: "Space", windowsVirtualKeyCode: 32 },
    "`": { code: "Backquote", windowsVirtualKeyCode: 192 },
    "-": { code: "Minus", windowsVirtualKeyCode: 189 },
    "=": { code: "Equal", windowsVirtualKeyCode: 187 },
    "[": { code: "BracketLeft", windowsVirtualKeyCode: 219 },
    "]": { code: "BracketRight", windowsVirtualKeyCode: 221 },
    "\\": { code: "Backslash", windowsVirtualKeyCode: 220 },
    ";": { code: "Semicolon", windowsVirtualKeyCode: 186 },
    "'": { code: "Quote", windowsVirtualKeyCode: 222 },
    ",": { code: "Comma", windowsVirtualKeyCode: 188 },
    ".": { code: "Period", windowsVirtualKeyCode: 190 },
    "/": { code: "Slash", windowsVirtualKeyCode: 191 }
  };
  const SHIFTED_SYMBOL_KEYS = {
    "~": { code: "Backquote", windowsVirtualKeyCode: 192 },
    _: { code: "Minus", windowsVirtualKeyCode: 189 },
    "+": { code: "Equal", windowsVirtualKeyCode: 187 },
    "{": { code: "BracketLeft", windowsVirtualKeyCode: 219 },
    "}": { code: "BracketRight", windowsVirtualKeyCode: 221 },
    "|": { code: "Backslash", windowsVirtualKeyCode: 220 },
    ":": { code: "Semicolon", windowsVirtualKeyCode: 186 },
    '"': { code: "Quote", windowsVirtualKeyCode: 222 },
    "<": { code: "Comma", windowsVirtualKeyCode: 188 },
    ">": { code: "Period", windowsVirtualKeyCode: 190 },
    "?": { code: "Slash", windowsVirtualKeyCode: 191 },
    "!": { code: "Digit1", windowsVirtualKeyCode: 49 },
    "@": { code: "Digit2", windowsVirtualKeyCode: 50 },
    "#": { code: "Digit3", windowsVirtualKeyCode: 51 },
    $: { code: "Digit4", windowsVirtualKeyCode: 52 },
    "%": { code: "Digit5", windowsVirtualKeyCode: 53 },
    "^": { code: "Digit6", windowsVirtualKeyCode: 54 },
    "&": { code: "Digit7", windowsVirtualKeyCode: 55 },
    "*": { code: "Digit8", windowsVirtualKeyCode: 56 },
    "(": { code: "Digit9", windowsVirtualKeyCode: 57 },
    ")": { code: "Digit0", windowsVirtualKeyCode: 48 }
  };
  function digitDescriptor(digit) {
    const vk = digit.charCodeAt(0);
    return {
      key: digit,
      code: `Digit${digit}`,
      windowsVirtualKeyCode: vk,
      text: digit
    };
  }
  function letterDescriptor(letter) {
    const upper = letter.toUpperCase();
    const vk = upper.charCodeAt(0);
    return {
      key: letter,
      code: `Key${upper}`,
      windowsVirtualKeyCode: vk,
      text: letter
    };
  }
  function resolveKey(key) {
    if (!key || typeof key !== "string") {
      throw new Error(
        `resolveKey: empty or non-string key (got ${JSON.stringify(key)})`
      );
    }
    if (NAMED_KEYS[key]) {
      return {
        ...NAMED_KEYS[key],
        isPrintable: NAMED_KEYS[key].text !== void 0
      };
    }
    if (key.length === 1) {
      const code = key.charCodeAt(0);
      if (code >= 65 && code <= 90 || code >= 97 && code <= 122) {
        return { ...letterDescriptor(key), isPrintable: true };
      }
      if (code >= 48 && code <= 57) {
        return { ...digitDescriptor(key), isPrintable: true };
      }
      if (code === 32) {
        return {
          key: " ",
          code: "Space",
          windowsVirtualKeyCode: 32,
          text: " ",
          isPrintable: true
        };
      }
      if (SYMBOL_KEYS[key]) {
        return {
          key,
          code: SYMBOL_KEYS[key].code,
          windowsVirtualKeyCode: SYMBOL_KEYS[key].windowsVirtualKeyCode,
          text: key,
          isPrintable: true
        };
      }
      if (SHIFTED_SYMBOL_KEYS[key]) {
        return {
          key,
          code: SHIFTED_SYMBOL_KEYS[key].code,
          windowsVirtualKeyCode: SHIFTED_SYMBOL_KEYS[key].windowsVirtualKeyCode,
          text: key,
          isPrintable: true
        };
      }
    }
    throw new Error(`resolveKey: unknown key "${key}"`);
  }
  const refToEl = /* @__PURE__ */ new Map();
  let nextRef = 0;
  function assignRef(el) {
    for (const [refId2, existing] of refToEl) {
      if (existing === el) {
        return refId2;
      }
    }
    nextRef++;
    const refId = `@e${nextRef}`;
    refToEl.set(refId, el);
    return refId;
  }
  function resolveRef(refId) {
    const el = refToEl.get(refId);
    if (!el) {
      throw new Error(
        `stale ref: ${refId} is not known on this page (it may have navigated or the ref was minted elsewhere)`
      );
    }
    if (!el.isConnected) {
      refToEl.delete(refId);
      throw new Error(
        `stale ref: ${refId} points to an element that is no longer in the document (page re-rendered or navigated)`
      );
    }
    return el;
  }
  function generateXPath(element) {
    const idXPath = generateIdXPath(element);
    if (idXPath) return idXPath;
    const segments = [];
    let current = element;
    while (current && current !== document.documentElement) {
      const segment = generateSegment(current);
      segments.unshift(segment);
      current = current.parentElement;
    }
    return `/${segments.join("/")}`;
  }
  function generateIdXPath(element) {
    const id = element.id;
    if (!id) return null;
    const matches = document.querySelectorAll(`#${CSS.escape(id)}`);
    if (matches.length !== 1) return null;
    const tagMatches = document.querySelectorAll(
      `${element.tagName.toLowerCase()}#${CSS.escape(id)}`
    );
    if (tagMatches.length === 1) {
      return `//${element.tagName.toLowerCase()}[@id="${id}"]`;
    }
    return `//*[@id="${id}"]`;
  }
  function generateSegment(element) {
    const tag = element.tagName.toLowerCase();
    if (!element.parentElement) {
      return tag;
    }
    const siblings = Array.from(element.parentElement.children).filter(
      (s) => s.tagName === element.tagName
    );
    if (siblings.length === 1) {
      return tag;
    }
    const index = siblings.indexOf(element) + 1;
    return `${tag}[${index}]`;
  }
  function findElement(target) {
    const elements = findAllElementsRaw(target);
    if (elements.length === 0) return null;
    const index = target.index ?? 0;
    return elements[index] ?? null;
  }
  function findAllElements(target) {
    return findAllElementsRaw(target);
  }
  function waitForElement(target, timeoutMs = 5e3, options = {}) {
    const { force = false, throwOnTimeout = false } = options;
    const existing = findElement(target);
    if (existing) return Promise.resolve(existing);
    if (!target.waitForAppear && !force) return Promise.resolve(null);
    return new Promise((resolve, reject) => {
      let resolved = false;
      const timer = setTimeout(() => {
        if (!resolved) {
          obs.disconnect();
          if (throwOnTimeout) {
            reject(
              new Error(
                `Element ${describeTarget(target)} did not appear within ${timeoutMs}ms`
              )
            );
          } else {
            resolve(null);
          }
        }
      }, timeoutMs);
      const poll = setInterval(() => {
        if (resolved) return;
        const el = findElement(target);
        if (el) {
          resolved = true;
          clearTimeout(timer);
          clearInterval(poll);
          obs.disconnect();
          if (throwOnTimeout) resolve(el);
          else resolve(el);
        }
      }, 50);
      const obs = new MutationObserver(() => {
        const el = findElement(target);
        if (el && !resolved) {
          resolved = true;
          clearTimeout(timer);
          obs.disconnect();
          clearInterval(poll);
          resolve(el);
        }
      });
      obs.observe(document.documentElement, {
        childList: true,
        subtree: true
      });
    });
  }
  function findAllElementsRaw(target) {
    let elements = [];
    if (target.ref) {
      const el = resolveRef(target.ref);
      return el ? [el] : [];
    }
    if (target.selector) {
      elements = querySelectorAllSafe(target.selector);
      return applyFilters(elements, target);
    }
    if (target.xpath) {
      elements = queryXPath(target.xpath);
      return applyFilters(elements, target);
    }
    if (target.id) {
      const el = document.getElementById(target.id);
      if (el) elements = [el];
      return applyFilters(elements, target);
    }
    if (target.name) {
      elements = querySelectorAllSafe(`[name="${CSS.escape(target.name)}"]`);
      if (elements.length > 0) return applyFilters(elements, target);
      elements = Array.from(document.querySelectorAll(`[name="${target.name}"]`));
      return applyFilters(elements, target);
    }
    if (target.role) {
      elements = querySelectorAllSafe(`[role="${target.role}"]`);
      if (elements.length > 0) return applyFilters(elements, target);
    }
    if (target.label) {
      elements = findByLabel(target.label);
      return applyFilters(elements, target);
    }
    if (target.placeholder) {
      elements = querySelectorAllSafe(
        `[placeholder="${CSS.escape(target.placeholder)}"]`
      );
      if (elements.length > 0) return applyFilters(elements, target);
    }
    if (target.text) {
      elements = findByText(
        target.text,
        target.tag,
        target.textMatch,
        target.caseSensitive
      );
      return applyFilters(elements, target);
    }
    if (target.tag) {
      let selector = target.tag;
      if (target.type) {
        selector += `[type="${target.type}"]`;
      }
      elements = querySelectorAllSafe(selector);
      return applyFilters(elements, target);
    }
    return [];
  }
  function querySelectorAllSafe(selector) {
    try {
      return Array.from(document.querySelectorAll(selector));
    } catch {
      return [];
    }
  }
  function queryXPath(xpath) {
    try {
      const result = document.evaluate(
        xpath,
        document,
        null,
        XPathResult.ORDERED_NODE_SNAPSHOT_TYPE,
        null
      );
      const elements = [];
      for (let i = 0; i < result.snapshotLength; i++) {
        const node = result.snapshotItem(i);
        if (node instanceof Element) {
          elements.push(node);
        }
      }
      return elements;
    } catch {
      return [];
    }
  }
  function findByLabel(labelText) {
    const labels = Array.from(document.querySelectorAll("label"));
    for (const label of labels) {
      if (getTextContent(label).trim() === labelText.trim()) {
        if (label.htmlFor) {
          const control = document.getElementById(label.htmlFor);
          if (control) return [control];
        }
        const nested = label.querySelector("input, select, textarea");
        if (nested) return [nested];
      }
    }
    const allControls = document.querySelectorAll("input, select, textarea");
    for (const control of allControls) {
      const labelledBy = control.getAttribute("aria-labelledby");
      if (labelledBy) {
        const labelEl = document.getElementById(labelledBy);
        if (labelEl && getTextContent(labelEl).trim() === labelText.trim()) {
          return [control];
        }
      }
      const ariaLabel = control.getAttribute("aria-label");
      if (ariaLabel && ariaLabel.trim() === labelText.trim()) {
        return [control];
      }
    }
    return [];
  }
  function findByText(text, tag, matchMode = "contains", caseSensitive = false) {
    const results = [];
    const selector = tag || "*";
    const elements = document.querySelectorAll(selector);
    for (const el of elements) {
      const textContent = getTextContent(el);
      if (matchText(textContent, text, matchMode, caseSensitive)) {
        results.push(el);
      }
    }
    return results;
  }
  function matchText(content, pattern, mode, caseSensitive) {
    const a = caseSensitive ? content : content.toLowerCase();
    const b = caseSensitive ? pattern : pattern.toLowerCase();
    switch (mode) {
      case "exact":
        return a === b;
      case "contains":
        return a.includes(b);
      case "startsWith":
        return a.startsWith(b);
      case "endsWith":
        return a.endsWith(b);
      case "regex":
        try {
          const flags = caseSensitive ? "" : "i";
          return new RegExp(pattern, flags).test(content);
        } catch {
          return false;
        }
      default:
        return a.includes(b);
    }
  }
  function applyFilters(elements, target) {
    let result = elements;
    if (target.visible) {
      result = result.filter(isVisible);
    }
    if (target.enabled) {
      result = result.filter(isEnabled);
    }
    return result;
  }
  function isVisible(element) {
    if (!element.ownerDocument) return false;
    const style = window.getComputedStyle(element);
    if (style.display === "none" || style.visibility === "hidden" || style.opacity === "0") {
      return false;
    }
    const rect = element.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return false;
    const viewportWidth = window.innerWidth || document.documentElement.clientWidth;
    const viewportHeight = window.innerHeight || document.documentElement.clientHeight;
    return rect.bottom > 0 && rect.right > 0 && rect.top < viewportHeight && rect.left < viewportWidth;
  }
  function isEnabled(element) {
    if (element instanceof HTMLInputElement) return !element.disabled;
    if (element instanceof HTMLSelectElement) return !element.disabled;
    if (element instanceof HTMLTextAreaElement) return !element.disabled;
    if (element instanceof HTMLButtonElement) return !element.disabled;
    if (element instanceof HTMLFieldSetElement) return !element.disabled;
    const ariaDisabled = element.getAttribute("aria-disabled");
    if (ariaDisabled === "true") return false;
    return true;
  }
  function getElementInfo(element) {
    const rect = element.getBoundingClientRect();
    const text = getTextContent(element);
    const attributes = {};
    for (const attr of Array.from(element.attributes)) {
      attributes[attr.name] = attr.value;
    }
    return {
      tag: element.tagName.toLowerCase(),
      text: text.substring(0, 500),
      // Limit text length
      selector: generateCSSSelector(element),
      xpath: generateXPath(element),
      type: getAttributeSafe(element, "type"),
      name: getAttributeSafe(element, "name"),
      id: element.id || void 0,
      className: element.className ? String(element.className).trim() : void 0,
      ariaLabel: getAttributeSafe(element, "aria-label"),
      value: element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement ? element.value : void 0,
      visible: isVisible(element),
      enabled: isEnabled(element),
      boundingBox: {
        x: rect.x,
        y: rect.y,
        width: rect.width,
        height: rect.height,
        top: rect.top,
        bottom: rect.bottom,
        left: rect.left,
        right: rect.right
      },
      attributes
    };
  }
  function getTextContent(element) {
    if (element instanceof HTMLInputElement) {
      if (element.type === "button" || element.type === "submit") {
        return element.value || element.textContent || "";
      }
      return element.value || "";
    }
    if (element instanceof HTMLTextAreaElement) {
      return element.value || "";
    }
    if (element instanceof HTMLSelectElement) {
      const selected = element.options[element.selectedIndex];
      return (selected == null ? void 0 : selected.textContent) || "";
    }
    const sources = [
      element.textContent,
      getAttributeSafe(element, "aria-label"),
      getAttributeSafe(element, "title"),
      getAttributeSafe(element, "alt")
    ];
    return sources.find((s) => s == null ? void 0 : s.trim()) || "";
  }
  function getAttributeSafe(element, name) {
    return element.getAttribute(name) || void 0;
  }
  function generateCSSSelector(element) {
    if (element.id) {
      return `#${CSS.escape(element.id)}`;
    }
    const parts = [];
    let current = element;
    while (current && current !== document.documentElement) {
      let selector = current.tagName.toLowerCase();
      if (current.id) {
        parts.unshift(`#${CSS.escape(current.id)}`);
        break;
      }
      if (current.className && typeof current.className === "string") {
        const classes = current.className.trim().split(/\s+/).filter((c) => c && !c.startsWith("__")).map((c) => `.${CSS.escape(c)}`).join("");
        selector += classes;
      }
      if (current.parentElement) {
        const currentTag = current.tagName;
        const siblings = Array.from(current.parentElement.children);
        const sameTag = siblings.filter((s) => s.tagName === currentTag);
        if (sameTag.length > 1) {
          const index = sameTag.indexOf(current) + 1;
          selector += `:nth-of-type(${index})`;
        }
      }
      parts.unshift(selector);
      current = current.parentElement;
    }
    return parts.join(" > ");
  }
  function describeTarget(target) {
    if (target.selector) return `"${target.selector}"`;
    if (target.xpath) return `xpath "${target.xpath}"`;
    if (target.id) return `#${target.id}`;
    if (target.text) return `text "${target.text}"`;
    if (target.name) return `[name="${target.name}"]`;
    if (target.role) return `[role="${target.role}"]`;
    return JSON.stringify(target);
  }
  async function waitForActionableElement(target, opts = {}) {
    const timeoutMs = opts.timeoutMs ?? 5e3;
    const requireEnabled = opts.requireEnabled ?? true;
    const deadline = Date.now() + timeoutMs;
    const check = (element) => {
      const info = getElementInfo(element);
      if (!info.visible) return { ok: false, reason: "is not visible" };
      if (requireEnabled && !info.enabled)
        return { ok: false, reason: "is disabled" };
      return { ok: true };
    };
    const immediate = findElement(target);
    if (immediate && check(immediate).ok) return immediate;
    return new Promise((resolve, reject) => {
      let settled = false;
      let foundElement = null;
      const label = describeTarget(target);
      const cleanup = () => {
        clearTimeout(timer);
        clearInterval(poll);
        obs.disconnect();
      };
      const succeed = (el) => {
        if (settled) return;
        settled = true;
        cleanup();
        resolve(el);
      };
      const fail = (reason) => {
        if (settled) return;
        settled = true;
        cleanup();
        reject(
          new Error(
            `Element ${label} ${reason} (waited ${timeoutMs}ms for it to become actionable)`
          )
        );
      };
      const evaluate = () => {
        if (settled) return;
        if (Date.now() > deadline) {
          if (!foundElement) fail("was not found");
          else {
            const result2 = check(foundElement);
            fail(result2.reason ?? "is not actionable");
          }
          return;
        }
        const el = findElement(target);
        if (!el) return;
        foundElement = el;
        const result = check(el);
        if (result.ok) succeed(el);
      };
      const timer = setTimeout(evaluate, timeoutMs);
      const obs = new MutationObserver(() => evaluate());
      obs.observe(document.documentElement, { childList: true, subtree: true });
      const poll = setInterval(evaluate, 50);
      evaluate();
    });
  }
  const EVALUATE_VIA_CDP = "evaluateViaCdp";
  const CDP_INPUT_RELAY_ACTIONS = /* @__PURE__ */ new Set([
    "click",
    "dblclick",
    "rightclick",
    "pressKey",
    "type"
  ]);
  function cdpInputAvailable() {
    return typeof chrome !== "undefined" && typeof chrome.debugger !== "undefined";
  }
  async function relayCdpInput(command) {
    const response = await new Promise((resolve) => {
      chrome.runtime.sendMessage(
        { type: "CDP_INPUT", command },
        (result) => {
          if (chrome.runtime.lastError) {
            resolve(null);
            return;
          }
          resolve(result ?? null);
        }
      );
    });
    if (!response) return null;
    if (!response.success && typeof response.error === "string" && response.error.includes("CDP_INPUT_UNSUPPORTED")) {
      return null;
    }
    return response;
  }
  async function executeCommand(command) {
    const start = Date.now();
    try {
      const data = await executeAction(command);
      return {
        id: command.id,
        success: true,
        data,
        duration: Date.now() - start,
        pageInfo: getPageInfo()
      };
    } catch (error) {
      return {
        id: command.id,
        success: false,
        error: error instanceof Error ? error.message : String(error),
        duration: Date.now() - start,
        pageInfo: getPageInfo()
      };
    }
  }
  function requireTarget(target, action) {
    if (!target) {
      throw new Error(
        `Action "${action}" requires a "target" selector in the command`
      );
    }
    return target;
  }
  function requireValue(value, action) {
    if (value === void 0) {
      throw new Error(`Action "${action}" requires a "value" in the command`);
    }
    return value;
  }
  function waitTimeout(options) {
    const t = options == null ? void 0 : options.timeout;
    if (typeof t !== "number" || t <= 0) return 5e3;
    return Math.min(t, 2e4);
  }
  async function executeAction(command) {
    const { action, target, value, options } = command;
    if (CDP_INPUT_RELAY_ACTIONS.has(action) && cdpInputAvailable()) {
      const relayed = await relayCdpInput(command);
      if (relayed) return relayed;
    }
    switch (action) {
      // ─── Finding / Inspection ─────────────────────────────────────
      case "find":
        return handleFind(requireTarget(target, action), options);
      case "findAll":
        return handleFindAll(requireTarget(target, action), options);
      case "wait":
        return handleWait(
          requireTarget(target, action),
          options == null ? void 0 : options.timeout
        );
      case "isVisible":
        return handleIsVisible(requireTarget(target, action));
      case "isEnabled":
        return handleIsEnabled(requireTarget(target, action));
      case "getValue":
        return handleGetValue(requireTarget(target, action));
      case "getAttribute":
        return handleGetAttribute(
          requireTarget(target, action),
          options == null ? void 0 : options.attribute
        );
      case "getText":
        return handleGetText(requireTarget(target, action));
      case "getHTML":
        return handleGetHTML(
          requireTarget(target, action),
          options == null ? void 0 : options.outer
        );
      case "getOuterHTML":
        return handleGetHTML(requireTarget(target, action), true);
      case "getBoundingBox":
        return handleGetBoundingBox(requireTarget(target, action));
      case "getComputedStyle":
        return handleGetComputedStyle(
          requireTarget(target, action),
          options == null ? void 0 : options.property
        );
      case "getPageInfo":
        return getPageInfo();
      case "xpath":
        return handleXPath(requireTarget(target, action));
      // ─── Interaction ──────────────────────────────────────────────
      case "click":
        return handleClick(
          requireTarget(target, action),
          options == null ? void 0 : options.button,
          options == null ? void 0 : options.count,
          waitTimeout(options)
        );
      case "dblclick":
        return handleClick(
          requireTarget(target, action),
          "left",
          2,
          waitTimeout(options)
        );
      case "rightclick":
        return handleClick(
          requireTarget(target, action),
          "right",
          1,
          waitTimeout(options)
        );
      case "hover":
        return handleHover(requireTarget(target, action), waitTimeout(options));
      case "focus":
        return handleFocus(requireTarget(target, action), waitTimeout(options));
      case "blur":
        return handleBlur(requireTarget(target, action), waitTimeout(options));
      case "scrollTo":
        return handleScrollTo(
          requireTarget(target, action),
          waitTimeout(options)
        );
      case "fill":
        return handleFill(
          requireTarget(target, action),
          requireValue(value, action),
          waitTimeout(options)
        );
      case "type":
        return handleType(
          requireTarget(target, action),
          requireValue(value, action),
          waitTimeout(options)
        );
      case "clear":
        return handleClear(requireTarget(target, action), waitTimeout(options));
      case "select":
        return handleSelect(
          requireTarget(target, action),
          requireValue(value, action),
          waitTimeout(options)
        );
      case "check":
        return handleCheck(
          requireTarget(target, action),
          true,
          waitTimeout(options)
        );
      case "uncheck":
        return handleCheck(
          requireTarget(target, action),
          false,
          waitTimeout(options)
        );
      case "pressKey":
        return handlePressKey(
          target,
          requireValue(value, action),
          waitTimeout(options)
        );
      case "selectText":
        return handleSelectText(
          requireTarget(target, action),
          waitTimeout(options)
        );
      // ─── Internal CDP preparation (invoked by the background) ─────
      // These never run on the user-facing path. The background's trusted
      // (CDP) click/key/type dispatchers call them to wait for the element,
      // scroll it into view, and (for keys) focus it — then report the
      // geometry / focus state the CDP dispatch needs.
      case "prepareClick":
        return handlePrepareClick(
          requireTarget(target, action),
          waitTimeout(options)
        );
      case "prepareKeys":
        return handlePrepareKeys(target, waitTimeout(options));
      // ─── Navigation ───────────────────────────────────────────────
      case "navigate":
        return handleNavigate(requireValue(value, action));
      case "reload":
        return handleReload();
      case "goBack":
        return handleGoBack();
      case "goForward":
        return handleGoForward();
      // ─── Screenshot ───────────────────────────────────────────────
      case "screenshot":
        return handleScreenshot();
      // ─── Script Execution ─────────────────────────────────────────
      // SECURITY: This action executes arbitrary JavaScript in the page context.
      // It is intended for browser automation use cases where the caller is trusted.
      // The server enforces authentication (IP whitelist + bearer token) before
      // forwarding commands to the extension.
      case "evaluate":
        return handleEvaluate(requireValue(value, action));
      case "fetch":
        return handleFetchViaBackground(requireValue(value, action), options);
      case "printToPDF":
        return handlePrintToPDF(options == null ? void 0 : options.tabId);
      // ─── Highlight ────────────────────────────────────────────────
      case "highlight":
        return handleHighlight(
          requireTarget(target, action),
          waitTimeout(options)
        );
      case "unhighlight":
        return handleUnhighlight();
      // ─── Tab Management ──────────────────────────────────────────
      case "listTabs":
        return handleListTabs();
      case "getTabInfo":
        return handleGetTabInfo(options == null ? void 0 : options.tabId);
      case "switchTab":
        return handleSwitchTab(requireValue(value, action));
      case "getSessionStorage":
        return sessionStorage.getItem(value || "eReceiptData");
      case "getLocalStorage":
        return localStorage.getItem(requireValue(value, action));
      case "fetchInPage":
        return handleFetchInPage(requireValue(value, action), options);
      case "fetchViaDOM":
        return handleFetchInPage(requireValue(value, action), options);
      case "fetchFromCS":
        return handleFetchFromCS(requireValue(value, action), options);
      case "openTab":
        return handleOpenTab(options);
      case "closeTab":
        return handleCloseTab((options == null ? void 0 : options.tabId) || Number(value));
      case "cdpNavigate":
        return handleCdpNavigate(
          options == null ? void 0 : options.tabId,
          requireValue(value, action)
        );
      case EVALUATE_VIA_CDP:
        return handleEvaluateViaCdp(requireValue(value, action));
      default:
        throw new Error(`Unknown action: ${action}`);
    }
  }
  function handleFind(target, options) {
    const element = findElement(target);
    if (!element) return null;
    const info = getElementInfo(element);
    if (options == null ? void 0 : options.assignRef) info.ref = assignRef(element);
    return info;
  }
  function handleFindAll(target, options) {
    const elements = findAllElements(target);
    return elements.map((el) => {
      const info = getElementInfo(el);
      if (options == null ? void 0 : options.assignRef) info.ref = assignRef(el);
      return info;
    });
  }
  async function handleWait(target, timeout) {
    const timeoutMs = timeout ?? 5e3;
    const element = await waitForElement(target, timeoutMs, {
      force: true,
      throwOnTimeout: true
    });
    if (!element) {
      const label = target.selector ?? target.xpath ?? JSON.stringify(target);
      throw new Error(
        `wait: element "${label}" did not appear within ${timeoutMs}ms`
      );
    }
    return getElementInfo(element);
  }
  function handleIsVisible(target) {
    const element = findElement(target);
    if (!element) return false;
    const info = getElementInfo(element);
    return info.visible ?? false;
  }
  function handleIsEnabled(target) {
    const element = findElement(target);
    if (!element) return false;
    const info = getElementInfo(element);
    return info.enabled ?? true;
  }
  function handleGetValue(target) {
    const element = findElement(target);
    if (!element) throw new Error("Element not found");
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
      return element.value;
    }
    if (element instanceof HTMLSelectElement) {
      return element.value;
    }
    if (element instanceof HTMLElement) {
      return element.textContent || "";
    }
    throw new Error("Cannot get value from this element type");
  }
  function handleGetAttribute(target, attribute) {
    const element = findElement(target);
    if (!element) throw new Error("Element not found");
    return element.getAttribute(attribute);
  }
  function handleGetText(target) {
    const element = findElement(target);
    if (!element) throw new Error("Element not found");
    return element.textContent || "";
  }
  function handleGetHTML(target, outer = false) {
    const element = findElement(target);
    if (!element) throw new Error("Element not found");
    if (outer) return element.outerHTML;
    return element.innerHTML;
  }
  function handleGetBoundingBox(target) {
    const element = findElement(target);
    if (!element) return null;
    const rect = element.getBoundingClientRect();
    return {
      x: rect.x,
      y: rect.y,
      width: rect.width,
      height: rect.height,
      top: rect.top,
      bottom: rect.bottom,
      left: rect.left,
      right: rect.right
    };
  }
  function handleGetComputedStyle(target, property) {
    const element = findElement(target);
    if (!element) return null;
    const style = window.getComputedStyle(element);
    return style.getPropertyValue(property);
  }
  function handleXPath(target) {
    const element = findElement(target);
    if (!element) return null;
    return generateXPath(element);
  }
  async function handleClick(target, button = "left", count = 1, timeoutMs = 5e3) {
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    element.scrollIntoView({ behavior: "auto", block: "center" });
    const rect = element.getBoundingClientRect();
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    const eventInit = {
      bubbles: true,
      cancelable: true,
      view: window,
      button: button === "right" ? 2 : 0,
      clientX: x,
      clientY: y
    };
    const pointerButton = button === "right" ? 2 : 0;
    const pointerInit = {
      bubbles: true,
      cancelable: true,
      view: window,
      pointerId: 1,
      pointerType: "mouse",
      button: pointerButton,
      buttons: pointerButton,
      clientX: x,
      clientY: y
    };
    const makePointer = (type) => typeof PointerEvent !== "undefined" ? new PointerEvent(type, pointerInit) : new MouseEvent(type, eventInit);
    const singleClickSequence = [
      "pointerover",
      "mouseover",
      "mouseenter",
      "pointerdown",
      "mousedown",
      "pointerup",
      "mouseup",
      "click"
    ];
    for (let i = 0; i < count; i++) {
      for (const eventType of singleClickSequence) {
        if (eventType.startsWith("pointer")) {
          element.dispatchEvent(makePointer(eventType));
        } else {
          element.dispatchEvent(
            new MouseEvent(eventType, eventInit)
          );
        }
      }
    }
    if (count === 2) {
      element.dispatchEvent(
        new MouseEvent("dblclick", eventInit)
      );
    }
  }
  async function handleHover(target, timeoutMs = 5e3) {
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: false
    });
    const rect = element.getBoundingClientRect();
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    const eventInit = {
      bubbles: true,
      cancelable: true,
      view: window,
      clientX: x,
      clientY: y
    };
    element.dispatchEvent(
      new MouseEvent("mouseover", eventInit)
    );
    element.dispatchEvent(
      new MouseEvent("mouseenter", eventInit)
    );
  }
  async function handleFocus(target, timeoutMs = 5e3) {
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: false
    });
    element.focus();
  }
  async function handleBlur(target, timeoutMs = 5e3) {
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: false
    });
    element.blur();
  }
  function waitForScrollSettle(maxMs = 500) {
    return new Promise((resolve) => {
      const start = Date.now();
      let lastTop = window.scrollY;
      let lastLeft = window.scrollX;
      let stableFrames = 0;
      const check = () => {
        const top = window.scrollY;
        const left = window.scrollX;
        if (top === lastTop && left === lastLeft) {
          stableFrames++;
        } else {
          stableFrames = 0;
          lastTop = top;
          lastLeft = left;
        }
        if (stableFrames >= 2 || Date.now() - start > maxMs) {
          resolve();
          return;
        }
        requestAnimationFrame(check);
      };
      requestAnimationFrame(check);
    });
  }
  async function handleScrollTo(target, timeoutMs = 5e3) {
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: false
    });
    element.scrollIntoView({ behavior: "auto", block: "center" });
    await waitForScrollSettle();
  }
  async function handleFill(target, value, timeoutMs = 5e3) {
    var _a;
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement || element instanceof HTMLSelectElement) {
      element.focus();
      if (element instanceof HTMLSelectElement) {
        element.value = value;
      } else {
        const nativeSetter = (_a = Object.getOwnPropertyDescriptor(
          element instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype,
          "value"
        )) == null ? void 0 : _a.set;
        if (nativeSetter) {
          nativeSetter.call(element, value);
        } else {
          element.value = value;
        }
      }
      element.dispatchEvent(new Event("input", { bubbles: true }));
      element.dispatchEvent(new Event("change", { bubbles: true }));
      element.dispatchEvent(new Event("blur", { bubbles: true }));
    } else {
      throw new Error("Element is not a form input");
    }
  }
  async function handleType(target, value, timeoutMs = 5e3) {
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
      element.focus();
      for (const char of value) {
        const charCode = (() => {
          try {
            return resolveKey(char).code;
          } catch {
            return `Key${char.toUpperCase()}`;
          }
        })();
        const keyDownEvent = new KeyboardEvent("keydown", {
          bubbles: true,
          key: char,
          code: charCode
        });
        const keyPressEvent = new KeyboardEvent("keypress", {
          bubbles: true,
          key: char,
          code: charCode
        });
        const inputEvent = new InputEvent("beforeinput", {
          bubbles: true,
          data: char,
          inputType: "insertText"
        });
        const afterInputEvent = new InputEvent("input", {
          bubbles: true,
          data: char,
          inputType: "insertText"
        });
        const keyUpEvent = new KeyboardEvent("keyup", {
          bubbles: true,
          key: char,
          code: charCode
        });
        element.dispatchEvent(keyDownEvent);
        element.dispatchEvent(keyPressEvent);
        element.dispatchEvent(inputEvent);
        element.value += char;
        element.dispatchEvent(afterInputEvent);
        element.dispatchEvent(keyUpEvent);
      }
      element.dispatchEvent(new Event("change", { bubbles: true }));
      element.dispatchEvent(new Event("blur", { bubbles: true }));
    } else {
      throw new Error("Element is not a text input");
    }
  }
  async function handleClear(target, timeoutMs = 5e3) {
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
      element.focus();
      element.value = "";
      element.dispatchEvent(new Event("input", { bubbles: true }));
      element.dispatchEvent(new Event("change", { bubbles: true }));
    } else {
      throw new Error("Element is not a text input");
    }
  }
  async function handleSelect(target, value, timeoutMs = 5e3) {
    var _a;
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    if (element instanceof HTMLSelectElement) {
      element.focus();
      const nativeSetter = (_a = Object.getOwnPropertyDescriptor(
        HTMLSelectElement.prototype,
        "value"
      )) == null ? void 0 : _a.set;
      if (nativeSetter) {
        nativeSetter.call(element, value);
      } else {
        element.value = value;
      }
      element.dispatchEvent(new Event("input", { bubbles: true }));
      element.dispatchEvent(new Event("change", { bubbles: true }));
    } else {
      throw new Error("Element is not a select");
    }
  }
  async function handleCheck(target, checked, timeoutMs = 5e3) {
    var _a;
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    if (element instanceof HTMLInputElement) {
      element.focus();
      const nativeSetter = (_a = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "checked"
      )) == null ? void 0 : _a.set;
      if (nativeSetter) {
        nativeSetter.call(element, checked);
      } else {
        element.checked = checked;
      }
      element.dispatchEvent(new Event("change", { bubbles: true }));
    } else {
      throw new Error("Element is not a checkbox/radio input");
    }
  }
  async function handlePressKey(target, key, timeoutMs = 5e3) {
    const element = target ? await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    }) : document.activeElement ?? document.body;
    element.focus();
    const descriptor = resolveKey(key);
    const keyInit = {
      bubbles: true,
      key: descriptor.key,
      code: descriptor.code,
      keyCode: descriptor.windowsVirtualKeyCode,
      which: descriptor.windowsVirtualKeyCode
    };
    const keyDownEvent = new KeyboardEvent("keydown", keyInit);
    const keyUpEvent = new KeyboardEvent("keyup", keyInit);
    element.dispatchEvent(keyDownEvent);
    element.dispatchEvent(keyUpEvent);
  }
  async function handlePrepareClick(target, timeoutMs = 5e3) {
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    element.scrollIntoView({ behavior: "auto", block: "center" });
    const rect = element.getBoundingClientRect();
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    return { x, y };
  }
  async function handlePrepareKeys(target, timeoutMs = 5e3) {
    if (!target) {
      return { focused: document.activeElement != null };
    }
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    element.scrollIntoView({ behavior: "auto", block: "center" });
    element.focus();
    return { focused: document.activeElement === element };
  }
  async function handleSelectText(target, timeoutMs = 5e3) {
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: false
    });
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
      element.select();
    } else {
      const selection = window.getSelection();
      if (selection) {
        const range = document.createRange();
        range.selectNodeContents(element);
        selection.removeAllRanges();
        selection.addRange(range);
      }
    }
  }
  function handleNavigate(url) {
    if (!url) throw new Error("URL is required");
    window.location.href = url;
  }
  function handleReload() {
    window.location.reload();
  }
  function handleGoBack() {
    window.history.back();
  }
  function handleGoForward() {
    window.history.forward();
  }
  function handleScreenshot(_target) {
    return "screenshot_requested";
  }
  function handleEvaluate(script) {
    if (!script) throw new Error("Script is required");
    let compiled;
    try {
      compiled = new Function(`return ( ${script} );`);
    } catch (err) {
      if (!(err instanceof SyntaxError)) throw err;
      try {
        compiled = new Function(
          `return (async () => { ${script} })();`
        );
      } catch (err2) {
        const msg = err2 instanceof Error ? err2.message : String(err2);
        throw new Error(
          msg.includes("SyntaxError") ? msg : `SyntaxError: ${msg}`
        );
      }
    }
    return (async () => compiled())();
  }
  async function handleEvaluateViaCdp(expression) {
    var _a;
    if (!expression) throw new Error("evaluateViaCdp: expression is required");
    if (typeof ((_a = chrome.runtime) == null ? void 0 : _a.sendMessage) !== "function") {
      throw new Error("evaluateViaCdp: chrome.runtime.sendMessage unavailable");
    }
    const tabId = await new Promise((resolve) => {
      chrome.runtime.sendMessage(
        { type: "GET_CURRENT_TAB_ID" },
        (response) => resolve((response == null ? void 0 : response.tabId) ?? 0)
      );
    });
    if (!tabId)
      throw new Error("evaluateViaCdp: could not resolve current tab id");
    return new Promise((resolve, reject) => {
      chrome.runtime.sendMessage(
        { type: "CDP_EVAL", tabId, expression },
        (resp) => {
          if (!resp) {
            reject(new Error("evaluateViaCdp: no response from background"));
            return;
          }
          if (!resp.ok) {
            reject(new Error(resp.error ?? "evaluateViaCdp: background error"));
            return;
          }
          resolve(resp.data);
        }
      );
    });
  }
  async function handlePrintToPDF(targetTabId) {
    let tabId = targetTabId;
    if (!tabId) {
      tabId = await new Promise((resolve) => {
        chrome.runtime.sendMessage(
          { type: "GET_TAB_ID" },
          (resp) => resolve(resp.tabId)
        );
      });
    }
    const response = await chrome.runtime.sendMessage({
      type: "PRINT_TO_PDF",
      tabId
    });
    if (!(response == null ? void 0 : response.ok)) throw new Error((response == null ? void 0 : response.error) || "printToPDF failed");
    return response.data;
  }
  async function handleFetchFromCS(url, options) {
    const method = (options == null ? void 0 : options.method) || "POST";
    const headers = {
      "Content-Type": "application/json",
      Accept: "application/json",
      "X-Channel-ID": "MYP_WEB",
      "X-Gateway-APIKey": AIA_API_KEY,
      "Content-Language": "en",
      ...(options == null ? void 0 : options.headers) ?? {}
    };
    try {
      const raw = localStorage.getItem("OAOP_LOGINDATA");
      if (raw) {
        const p = JSON.parse(raw);
        if (p.jwt) headers.Authorization = `Bearer ${p.jwt}`;
      }
    } catch {
    }
    const body = options == null ? void 0 : options.body;
    const resp = await fetch(url, {
      method,
      headers,
      credentials: "include",
      body: body !== void 0 ? JSON.stringify(body) : void 0
    });
    const status = resp.status;
    const contentType = resp.headers.get("content-type") || "";
    if (!resp.ok) {
      const errText = await resp.text();
      throw new Error(`HTTP ${status}: ${errText.slice(0, 400)}`);
    }
    const buf = await resp.arrayBuffer();
    const bytes = new Uint8Array(buf);
    let bin = "";
    for (let i = 0; i < bytes.length; i += 8192) {
      bin += String.fromCharCode(
        ...bytes.subarray(i, Math.min(i + 8192, bytes.length))
      );
    }
    return { status, contentType, base64: btoa(bin) };
  }
  const FETCH_IN_PAGE_ALLOWED_HOSTS = /* @__PURE__ */ new Set([
    "api.aia.com.my",
    "www.aia.com.my"
  ]);
  async function handleFetchInPage(url, options) {
    const parsed = new URL(url);
    if (parsed.protocol !== "https:" || !FETCH_IN_PAGE_ALLOWED_HOSTS.has(parsed.hostname)) {
      throw new Error(`fetchInPage: URL not in allowlist: ${parsed.hostname}`);
    }
    const jwtRaw = localStorage.getItem("OAOP_LOGINDATA");
    let jwt = "";
    try {
      const parsed2 = JSON.parse(jwtRaw || "{}");
      jwt = parsed2.jwt || "";
    } catch {
    }
    const headers = {
      "Content-Type": "application/json",
      Accept: "application/json",
      "X-Channel-ID": "MYP_WEB",
      "X-Gateway-APIKey": AIA_API_KEY,
      "Content-Language": "en",
      ...jwt ? { Authorization: `Bearer ${jwt}` } : {},
      ...(options == null ? void 0 : options.headers) ?? {}
    };
    const msgId = `__aip_${Date.now()}_${Math.random().toString(36).slice(2)}`;
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        window.removeEventListener("message", listener);
        reject(new Error("fetchInPage timeout after 30s"));
      }, 3e4);
      function listener(event) {
        var _a;
        if (event.source !== window || ((_a = event.data) == null ? void 0 : _a.__id) !== msgId)
          return;
        clearTimeout(timeout);
        window.removeEventListener("message", listener);
        const d = event.data;
        if (!d.ok) reject(new Error(d.error ?? "fetchInPage failed"));
        else resolve(d.data);
      }
      window.addEventListener("message", listener);
      const fetchBody = (options == null ? void 0 : options.body) !== void 0 ? JSON.stringify(options.body) : void 0;
      const scriptCode = `(async () => {
  const _id = ${JSON.stringify(msgId)};
  try {
    const r = await fetch(${JSON.stringify(url)}, {
      method: ${JSON.stringify((options == null ? void 0 : options.method) || "POST")},
      headers: ${JSON.stringify(headers)},
      credentials: "include",
      body: ${fetchBody !== void 0 ? JSON.stringify(fetchBody) : "undefined"},
    });
    const text = await r.text();
    if (!r.ok) throw new Error("HTTP " + r.status + ": " + text.slice(0, 200));
    let data; try { data = JSON.parse(text); } catch { data = text; }
    window.postMessage({ __id: _id, ok: true, data }, "*");
  } catch(e) {
    window.postMessage({ __id: _id, ok: false, error: e.message }, "*");
  }
})();`;
      const script = document.createElement("script");
      script.textContent = scriptCode;
      document.head.appendChild(script);
      document.head.removeChild(script);
    });
  }
  async function handleFetchViaBackground(url, options) {
    const AIA_API_ORIGINS = /* @__PURE__ */ new Set([
      "https://api.aia.com.my",
      "https://myaia.aia.com.my"
    ]);
    const defaultHeaders = {
      "Content-Type": "application/json",
      Accept: "application/json",
      "X-Channel-ID": "MYP_WEB",
      "X-Request-ID": `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
      "Content-Language": "en"
    };
    try {
      const targetOrigin = new URL(url).origin;
      if (AIA_API_ORIGINS.has(targetOrigin)) {
        const raw = localStorage.getItem("OAOP_LOGINDATA");
        if (raw) {
          const parsed = JSON.parse(raw);
          if (parsed.jwt) defaultHeaders.Authorization = `Bearer ${parsed.jwt}`;
        }
      }
    } catch {
    }
    const response = await chrome.runtime.sendMessage({
      type: "FETCH_URL",
      url,
      method: (options == null ? void 0 : options.method) || "POST",
      headers: (options == null ? void 0 : options.headers) ?? defaultHeaders,
      body: options == null ? void 0 : options.body
    });
    if (!(response == null ? void 0 : response.ok))
      throw new Error(
        (response == null ? void 0 : response.error) || `Fetch failed: HTTP ${response == null ? void 0 : response.status}`
      );
    return response.data;
  }
  async function handleHighlight(target, timeoutMs = 5e3) {
    const element = await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: false
    });
    window.dispatchEvent(
      new CustomEvent("htrncontrol:highlight", {
        detail: { element }
      })
    );
    return getElementInfo(element);
  }
  function handleUnhighlight() {
    window.dispatchEvent(new CustomEvent("htrncontrol:unhighlight"));
  }
  function safeHistoryLength() {
    var _a;
    try {
      const length = (_a = window.history) == null ? void 0 : _a.length;
      return typeof length === "number" ? length : 0;
    } catch (err) {
      if (err instanceof TypeError) return 0;
      console.warn("[HTR NControl] safeHistoryLength: unexpected error:", err);
      return 0;
    }
  }
  function getPageInfo() {
    return {
      url: window.location.href,
      title: document.title,
      domain: window.location.hostname,
      readyState: document.readyState,
      scrollX: window.scrollX,
      scrollY: window.scrollY,
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight,
      documentHeight: document.documentElement.scrollHeight,
      documentWidth: document.documentElement.scrollWidth,
      // happy-dom's `window.history` getter can throw a TypeError when the
      // browser frame isn't fully initialized. Guard so the rest of the
      // PageInfo still comes through (the goBack/goForward pre-check only
      // uses this as a hint; a missing value is fine — the runtime race is
      // authoritative).
      historyLength: safeHistoryLength()
    };
  }
  async function handleOpenTab(opts) {
    const response = await chrome.runtime.sendMessage({
      type: "OPEN_TAB",
      url: opts.url,
      sessionData: opts.sessionData
    });
    if (!(response == null ? void 0 : response.ok)) throw new Error((response == null ? void 0 : response.error) || "openTab failed");
    return { tabId: response.tabId };
  }
  async function handleCloseTab(tabId) {
    const response = await chrome.runtime.sendMessage({
      type: "CLOSE_TAB",
      tabId
    });
    if (!(response == null ? void 0 : response.ok)) throw new Error((response == null ? void 0 : response.error) || "closeTab failed");
  }
  async function handleCdpNavigate(tabId, url) {
    const response = await chrome.runtime.sendMessage({
      type: "CDP_NAVIGATE",
      tabId,
      url
    });
    if (!(response == null ? void 0 : response.ok)) throw new Error((response == null ? void 0 : response.error) || "cdpNavigate failed");
  }
  async function handleListTabs() {
    const response = await chrome.runtime.sendMessage({ type: "GET_TABS_INFO" });
    if (!(response == null ? void 0 : response.success)) {
      throw new Error((response == null ? void 0 : response.error) || "Failed to list tabs");
    }
    return response.tabs;
  }
  async function handleGetTabInfo(tabId) {
    if (tabId === void 0) {
      const response = await chrome.runtime.sendMessage({
        type: "GET_CURRENT_TAB_ID"
      });
      if (!(response == null ? void 0 : response.tabId)) {
        throw new Error("No current tab available");
      }
      tabId = response.tabId;
    }
    const tabsResponse = await chrome.runtime.sendMessage({
      type: "GET_TABS_INFO"
    });
    if (!(tabsResponse == null ? void 0 : tabsResponse.success)) {
      throw new Error((tabsResponse == null ? void 0 : tabsResponse.error) || "Failed to get tab info");
    }
    const tab = tabsResponse.tabs.find((t) => t.id === tabId);
    if (!tab) {
      throw new Error(`Tab ${tabId} not found`);
    }
    return tab;
  }
  async function handleSwitchTab(tabIdStr) {
    const tabId = Number(tabIdStr);
    if (Number.isNaN(tabId)) {
      throw new Error(`Invalid tab ID: ${tabIdStr}`);
    }
    const response = await chrome.runtime.sendMessage({
      type: "SWITCH_TAB",
      tabId
    });
    if (!(response == null ? void 0 : response.success)) {
      throw new Error((response == null ? void 0 : response.error) || `Failed to switch to tab ${tabId}`);
    }
    return { success: true };
  }
  window.__htrcliDom = {
    exec: (command) => executeCommand(command),
    version: 1
  };
})();
