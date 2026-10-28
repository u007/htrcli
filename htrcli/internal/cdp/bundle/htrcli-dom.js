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
  class DetachedRefError extends Error {
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
      throw new DetachedRefError(
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
          resolved = true;
          clearInterval(poll);
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
  function isOnlyOffscreen(element) {
    if (!element.ownerDocument) return false;
    const style = window.getComputedStyle(element);
    if (style.display === "none" || style.visibility === "hidden" || style.opacity === "0") {
      return false;
    }
    const rect = element.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return false;
    return !isVisible(element);
  }
  function isRendered(element) {
    const check = element.checkVisibility;
    if (typeof check !== "function") {
      throw new Error(
        "visibleOnly requires Element.checkVisibility (Chrome 105+, Firefox 125+)"
      );
    }
    return check.call(element, { checkOpacity: true, checkVisibilityCSS: true });
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
  function waitForActionableElement(target, opts = {}) {
    return waitForActionable(target, opts, false);
  }
  function waitForScrollableActionableElement(target, opts = {}) {
    return waitForActionable(target, opts, true);
  }
  async function waitForActionable(target, opts, scrollOffscreen) {
    const timeoutMs = opts.timeoutMs ?? 5e3;
    const requireEnabled = opts.requireEnabled ?? true;
    const deadline = Date.now() + timeoutMs;
    const scrolled = /* @__PURE__ */ new WeakSet();
    const attemptScroll = (element) => {
      if (!scrollOffscreen) return;
      if (scrolled.has(element)) return;
      if (isOnlyOffscreen(element)) {
        scrolled.add(element);
        element.scrollIntoView({ behavior: "instant", block: "center" });
      }
    };
    const check = (element) => {
      const info = getElementInfo(element);
      if (!info.visible) return { ok: false, reason: "is not visible" };
      if (requireEnabled && !info.enabled)
        return { ok: false, reason: "is disabled" };
      return { ok: true };
    };
    const immediate = findElement(target);
    if (immediate) {
      attemptScroll(immediate);
      if (check(immediate).ok) return immediate;
    }
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
        attemptScroll(el);
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
  const DEFAULT_MAX_STEPS = 200;
  const DEFAULT_STEP_DELAY_MS = 120;
  function isScrollable(el) {
    const anyEl = el;
    if (typeof anyEl.scrollHeight !== "number") return false;
    if (anyEl.scrollHeight <= anyEl.clientHeight) return false;
    const style = window.getComputedStyle(anyEl);
    const overflowY = style.overflowY;
    return overflowY === "auto" || overflowY === "scroll" || overflowY === "overlay";
  }
  const MAX_ANCESTOR_WALK = 13;
  function findScrollContainer(first) {
    if (first) {
      let node = first;
      for (let i = 0; i < MAX_ANCESTOR_WALK && node; i++) {
        if (isScrollable(node)) return node;
        node = node.parentElement;
      }
    }
    const doc = document.scrollingElement ?? document.documentElement;
    if (doc && doc.scrollHeight > doc.clientHeight) return doc;
    return null;
  }
  function elementKey(el) {
    var _a, _b, _c;
    const anyEl = el;
    const keyAttr = ((_a = anyEl.getAttribute) == null ? void 0 : _a.call(anyEl, "data-item-id")) ?? ((_b = anyEl.getAttribute) == null ? void 0 : _b.call(anyEl, "data-msg-id")) ?? ((_c = anyEl.getAttribute) == null ? void 0 : _c.call(anyEl, "data-id")) ?? "";
    if (keyAttr) return { key: `k:${keyAttr}`, strategy: "key" };
    const text = (el.textContent ?? "").trim().slice(0, 120);
    if (text) return { key: `t:${text}`, strategy: "text" };
    return { key: null, strategy: "index" };
  }
  function sleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
  async function collectAll(target, options = {}) {
    const maxSteps = options.maxSteps ?? DEFAULT_MAX_STEPS;
    const stepDelayMs = options.stepDelayMs ?? DEFAULT_STEP_DELAY_MS;
    const maxItems = options.maxItems ?? 0;
    const onRow = options.onRow;
    const visibleOnly = options.visibleOnly === true;
    const seen = /* @__PURE__ */ new Map();
    const nodes = [];
    const nodeIds = /* @__PURE__ */ new WeakMap();
    let nodeCount = 0;
    const nodeKey = (el) => {
      const existing = nodeIds.get(el);
      if (existing !== void 0) return existing;
      nodeCount++;
      const created = `n:${nodeCount}`;
      nodeIds.set(el, created);
      return created;
    };
    let truncatedBy = null;
    let steps = 0;
    let scrolled = false;
    let keyStrategy = "key";
    const harvest = () => {
      const elements2 = findAllElements(target);
      for (const el of elements2) {
        if (visibleOnly && !isRendered(el)) continue;
        if (maxItems > 0 && seen.size >= maxItems) {
          truncatedBy = "maxItems";
          break;
        }
        const identity = elementKey(el);
        const key = identity.key === null ? nodeKey(el) : identity.key;
        if (identity.strategy === "index") {
          keyStrategy = "index";
        } else if (identity.strategy === "text" && keyStrategy === "key") {
          keyStrategy = "text";
        }
        if (seen.has(key)) continue;
        const info = getElementInfo(el);
        onRow == null ? void 0 : onRow(el, info);
        seen.set(key, info);
        nodes.push(el);
      }
      return elements2.length;
    };
    harvest();
    const container = findScrollContainer(findAllElements(target)[0]);
    if (!container) {
      return {
        elements: [...seen.values()],
        nodes,
        steps: 0,
        truncated: false,
        truncatedBy: null,
        scrolled: false,
        keyStrategy
      };
    }
    const anyContainer = container;
    let previousScrollTop = anyContainer.scrollTop;
    let previousHeight = anyContainer.scrollHeight;
    let unchanged = 0;
    const originalScrollTop = previousScrollTop;
    try {
      for (; steps < maxSteps; steps++) {
        if (maxItems > 0 && seen.size >= maxItems) {
          truncatedBy = "maxItems";
          break;
        }
        const before = seen.size;
        anyContainer.scrollTop = anyContainer.scrollTop + anyContainer.clientHeight;
        await sleep(stepDelayMs);
        harvest();
        const currentScrollTop = anyContainer.scrollTop;
        const currentHeight = anyContainer.scrollHeight;
        scrolled = true;
        if (seen.size === before && currentScrollTop === previousScrollTop && currentHeight === previousHeight) {
          unchanged++;
          if (unchanged >= 2) break;
        } else {
          unchanged = 0;
        }
        previousScrollTop = currentScrollTop;
        previousHeight = currentHeight;
      }
    } finally {
      if (!onRow) anyContainer.scrollTop = originalScrollTop;
    }
    if (truncatedBy === null && steps >= maxSteps && unchanged === 0) {
      truncatedBy = "maxSteps";
    }
    const elements = [...seen.values()];
    return {
      elements,
      nodes,
      steps,
      truncated: truncatedBy !== null,
      truncatedBy,
      scrolled,
      keyStrategy
    };
  }
  function textOf(el) {
    if (!el) return "";
    return (el.textContent ?? "").trim();
  }
  function firstTable(root) {
    if (root instanceof HTMLTableElement) return root;
    const found = root.querySelector("table");
    return found instanceof HTMLTableElement ? found : null;
  }
  function extractTable(root) {
    const table = firstTable(root);
    if (!table) {
      return { columns: [], rows: [], rowCount: 0 };
    }
    let headerCells = [];
    let headerRow = null;
    const thead = table.querySelector("thead");
    if (thead) {
      const tr = thead.querySelector("tr");
      if (tr) {
        headerCells = Array.from(tr.querySelectorAll("th, td"));
        headerRow = tr;
      }
    }
    if (headerCells.length === 0) {
      const firstTr = Array.from(table.querySelectorAll("tr")).find(
        (tr) => !(thead == null ? void 0 : thead.contains(tr))
      );
      const cells = firstTr ? Array.from(firstTr.querySelectorAll("td, th")) : [];
      if (firstTr && cells.length > 0 && cells.every((c) => {
        var _a;
        return ((_a = c.tagName) == null ? void 0 : _a.toUpperCase()) === "TH";
      })) {
        headerCells = cells;
        headerRow = firstTr;
      }
    }
    const bodyRows = [];
    const tbody = table.querySelector("tbody");
    if (tbody) {
      for (const tr of Array.from(tbody.querySelectorAll("tr"))) {
        if (tr === headerRow) continue;
        bodyRows.push(tr);
      }
    } else {
      for (const tr of Array.from(table.querySelectorAll("tr"))) {
        if (tr === headerRow) continue;
        if (thead == null ? void 0 : thead.contains(tr)) continue;
        bodyRows.push(tr);
      }
    }
    const seenNames = /* @__PURE__ */ new Map();
    let columns;
    if (headerCells.length > 0) {
      columns = headerCells.map((c, i) => {
        const base = textOf(c) || `col${i + 1}`;
        const seen = seenNames.get(base) ?? 0;
        seenNames.set(base, seen + 1);
        return seen === 0 ? base : `${base}_${seen + 1}`;
      });
    } else {
      const width = bodyRows.reduce(
        (max, tr) => Math.max(max, tr.querySelectorAll("td, th").length),
        0
      );
      columns = Array.from({ length: width }, (_, i) => `col${i + 1}`);
    }
    const rows = [];
    for (const tr of bodyRows) {
      const cells = Array.from(tr.querySelectorAll("td, th"));
      if (cells.length === 0) continue;
      const row = {};
      cells.forEach((cell, i) => {
        const col = columns[i] ?? `col${i + 1}`;
        row[col] = textOf(cell);
      });
      rows.push(row);
    }
    return { columns, rows, rowCount: rows.length };
  }
  const CONTAINER_TAGS = /* @__PURE__ */ new Set([
    "ul",
    "ol",
    "table",
    "tbody",
    "thead",
    "dl",
    "select"
  ]);
  function isContainerElement(el) {
    return CONTAINER_TAGS.has(el.tagName.toLowerCase());
  }
  function extractListFrom(roots, fields, options) {
    let items = [];
    if (options == null ? void 0 : options.container) {
      items = roots.flatMap((root) => listItemsOf(root));
    } else if (roots.length > 1) {
      items = roots;
    } else if (roots.length === 1) {
      items = isContainerElement(roots[0]) ? listItemsOf(roots[0]) : [roots[0]];
    }
    if (options == null ? void 0 : options.visibleOnly) {
      items = items.filter(isRendered);
    }
    const fieldPlans = fields && Object.keys(fields).length > 0 ? Object.entries(fields).map(([name, sel]) => ({
      name,
      sel,
      alternatives: alternativesOf(name, sel)
    })) : [];
    const out = [];
    for (const item of items) {
      if (fieldPlans.length > 0) {
        const row = {};
        for (const plan of fieldPlans) {
          row[plan.name] = readFieldValue(
            item,
            plan.alternatives,
            plan.sel,
            plan.name
          );
        }
        out.push(row);
      } else {
        out.push({ text: textOf(item) });
      }
    }
    return { items: out, count: out.length };
  }
  function alternativesOf(name, sel) {
    const alternatives = splitFieldAlternatives(sel);
    if (alternatives.length === 0) {
      throw new Error(`invalid selector for field "${name}": ${sel} (empty)`);
    }
    return alternatives;
  }
  function readFieldValue(item, alternatives, sel, name) {
    try {
      for (const alternative of alternatives) {
        const value = readFieldValueUnsafe(item, alternative);
        if (value !== "") return value;
      }
      return "";
    } catch (err) {
      const detail = err instanceof Error ? err.message : String(err);
      throw new Error(
        `invalid selector for field "${name}": ${sel} (${detail})`,
        {
          cause: err
        }
      );
    }
  }
  function splitFieldAlternatives(sel) {
    const parts = [];
    let current = "";
    let quote = "";
    for (let i = 0; i < sel.length; i++) {
      const ch = sel[i];
      if (quote !== "") {
        current += ch;
        if (ch === "\\" && i + 1 < sel.length) {
          i++;
          current += sel[i];
        } else if (ch === quote) {
          quote = "";
        }
        continue;
      }
      if (ch === '"' || ch === "'") {
        quote = ch;
        current += ch;
        continue;
      }
      if (ch === "|" && sel[i + 1] === "|") {
        parts.push(current);
        current = "";
        i++;
        continue;
      }
      current += ch;
    }
    parts.push(current);
    return parts.map((part) => part.trim()).filter((part) => part !== "");
  }
  function readFieldValueUnsafe(item, sel) {
    const attr = parseAttributeField(sel);
    if (attr) {
      const el = findFieldElement(item, attr.selector);
      if (!el) return "";
      return el.getAttribute(attr.attribute) ?? "";
    }
    const pureAttr = parsePureAttributeSelector(sel);
    if (pureAttr) {
      const el = findFieldElement(item, "");
      const found2 = el ? el.querySelector(sel) : null;
      const target = found2 ?? ((el == null ? void 0 : el.matches(sel)) ? el : null);
      return target ? target.getAttribute(pureAttr) ?? "" : "";
    }
    const found = item.querySelector(sel);
    if (found) return textOf(found);
    if (item.matches(sel)) return textOf(item);
    return "";
  }
  function parseAttributeField(sel) {
    if (!sel.startsWith("@")) return null;
    const rest = sel.slice(1);
    const at = rest.lastIndexOf("@");
    if (at === -1) {
      return { selector: "", attribute: rest };
    }
    return { selector: rest.slice(0, at), attribute: rest.slice(at + 1) };
  }
  function parsePureAttributeSelector(sel) {
    const m = /^\[([A-Za-z0-9_:.-]+)\]$/.exec(sel);
    return m ? m[1] : null;
  }
  function findFieldElement(item, selector) {
    if (selector === "") return item;
    return item.querySelector(selector);
  }
  function listItemsOf(root) {
    if (root instanceof HTMLUListElement || root instanceof HTMLOListElement) {
      const lis2 = Array.from(root.children).filter(
        (c) => c instanceof HTMLLIElement
      );
      if (lis2.length > 0) return lis2;
      return Array.from(root.children);
    }
    const lis = root.querySelectorAll(":scope > li");
    if (lis.length > 0) {
      return Array.from(lis);
    }
    return Array.from(root.children);
  }
  const PRESENTATION_ROLES = /* @__PURE__ */ new Set(["none", "presentation"]);
  const HEADING_TAGS = /* @__PURE__ */ new Set(["h1", "h2", "h3", "h4", "h5", "h6"]);
  const LANDMARK_TAGS = /* @__PURE__ */ new Map([
    ["main", "main"],
    ["nav", "navigation"],
    ["header", "banner"],
    ["footer", "contentinfo"],
    ["aside", "complementary"],
    ["form", "form"],
    ["section", "region"],
    ["article", "article"]
  ]);
  function getSnapshot() {
    const rootChildren = snapshotChildren(
      document.body ?? document.documentElement
    );
    return {
      role: "document",
      name: normalizeText(document.title),
      children: rootChildren.length > 0 ? rootChildren : void 0
    };
  }
  function snapshotChildren(parent) {
    const nodes = [];
    for (const child of Array.from(parent.children)) {
      nodes.push(...snapshotElement(child));
    }
    return nodes;
  }
  function snapshotElement(element) {
    if (isHidden(element)) {
      return [];
    }
    const children = snapshotChildren(element);
    const role = getRole(element);
    if (PRESENTATION_ROLES.has(role)) {
      return children;
    }
    const name = getAccessibleName(element, role);
    const value = getValue(element);
    const state = getState(element);
    const focusable = isFocusable(element);
    const interesting = role !== "generic" || Boolean(name) || Boolean(value) || Boolean(state && Object.keys(state).length > 0) || focusable;
    if (!interesting) {
      return children;
    }
    const node = { role };
    if (name) node.name = name;
    if (value) node.value = value;
    if (state && Object.keys(state).length > 0) node.state = state;
    if (shouldMintRef(element, role, focusable)) node.ref = assignRef(element);
    if (children.length > 0) node.children = children;
    return [node];
  }
  function shouldMintRef(element, role, focusable) {
    if (role === "text" || role === "document") return false;
    return focusable || role !== "generic" || isInteractiveElement(element);
  }
  function isInteractiveElement(element) {
    const tag = element.tagName.toLowerCase();
    if (tag === "button" || tag === "select" || tag === "textarea") return true;
    if (tag === "input") return true;
    if (tag === "summary") return true;
    if (tag === "a" && element.hasAttribute("href")) return true;
    if (tag === "option") return true;
    if (tag === "audio" || tag === "video")
      return element.hasAttribute("controls");
    return false;
  }
  function isFocusable(element) {
    if (!(element instanceof HTMLElement)) return false;
    if (element.hasAttribute("disabled")) return false;
    return element.tabIndex >= 0;
  }
  function isHidden(element) {
    if (!(element instanceof HTMLElement)) return false;
    if (element.hidden) return true;
    if (element.hasAttribute("inert")) return true;
    if (element.getAttribute("aria-hidden") === "true") return true;
    const style = window.getComputedStyle(element);
    return style.display === "none" || style.visibility === "hidden" || style.visibility === "collapse";
  }
  function getRole(element) {
    const explicit = normalizeRole(element.getAttribute("role"));
    if (explicit) return explicit;
    const tag = element.tagName.toLowerCase();
    if (HEADING_TAGS.has(tag)) return "heading";
    const landmark = LANDMARK_TAGS.get(tag);
    if (landmark) return landmark;
    switch (tag) {
      case "button":
        return "button";
      case "a":
        return element.hasAttribute("href") ? "link" : "generic";
      case "input":
        return getInputRole(element);
      case "textarea":
        return "textbox";
      case "select":
        return element.multiple ? "listbox" : "combobox";
      case "img":
        return "img";
      case "ul":
      case "ol":
        return "list";
      case "li":
        return "listitem";
      case "table":
        return "table";
      case "thead":
        return "rowgroup";
      case "tbody":
        return "rowgroup";
      case "tr":
        return "row";
      case "th":
        return "columnheader";
      case "td":
        return "cell";
      case "details":
        return "group";
      case "summary":
        return "button";
      case "dialog":
        return "dialog";
      default:
        return "generic";
    }
  }
  function getInputRole(input) {
    switch (input.type) {
      case "button":
      case "submit":
      case "reset":
      case "image":
        return "button";
      case "checkbox":
        return "checkbox";
      case "radio":
        return "radio";
      case "range":
        return "slider";
      case "search":
        return "searchbox";
      default:
        return "textbox";
    }
  }
  function normalizeRole(role) {
    if (!role) return "";
    return role.trim().toLowerCase();
  }
  function getAccessibleName(element, role) {
    const ariaLabel = normalizeText(element.getAttribute("aria-label"));
    if (ariaLabel) return ariaLabel;
    const labelledBy = normalizeText(element.getAttribute("aria-labelledby"));
    if (labelledBy) {
      const ids = labelledBy.split(/\s+/).filter(Boolean);
      const parts = [];
      for (const id of ids) {
        const label = document.getElementById(id);
        const text2 = normalizeText(label == null ? void 0 : label.textContent);
        if (text2) parts.push(text2);
      }
      if (parts.length > 0) return parts.join(" ");
    }
    const labelText = getAssociatedLabelText(element);
    if (labelText) return labelText;
    const alt = normalizeText(element.getAttribute("alt"));
    if (alt && (role === "img" || element.tagName.toLowerCase() === "img")) {
      return alt;
    }
    const title = normalizeText(element.getAttribute("title"));
    if (title) return title;
    const value = getValue(element);
    if (value) return value;
    const text = role === "generic" ? directTextContent(element) : normalizeText(element.textContent);
    return text;
  }
  function getAssociatedLabelText(element) {
    if (!(element instanceof HTMLElement)) return "";
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement || element instanceof HTMLSelectElement) {
      if (element.id) {
        for (const label of Array.from(document.querySelectorAll("label"))) {
          if (label instanceof HTMLLabelElement && label.htmlFor === element.id) {
            const labelText = normalizeText(label.textContent);
            if (labelText) return labelText;
          }
        }
      }
      const parentLabel = element.closest("label");
      const parentText = normalizeText(parentLabel == null ? void 0 : parentLabel.textContent);
      if (parentText) return parentText;
    }
    return "";
  }
  function getValue(element) {
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
      return normalizeText(element.value);
    }
    if (element instanceof HTMLSelectElement) {
      const option = element.options[element.selectedIndex];
      return normalizeText(option == null ? void 0 : option.textContent);
    }
    if (element instanceof HTMLProgressElement) {
      return normalizeText(element.value.toString());
    }
    return "";
  }
  function getState(element) {
    const state = {};
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement || element instanceof HTMLSelectElement || element instanceof HTMLButtonElement) {
      if (element.disabled) state.disabled = true;
    }
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
      if (element.readOnly) state.readonly = true;
    }
    const ariaDisabled = element.getAttribute("aria-disabled");
    if (ariaDisabled === "true") state.disabled = true;
    const ariaExpanded = element.getAttribute("aria-expanded");
    if (ariaExpanded === "true") state.expanded = true;
    const ariaSelected = element.getAttribute("aria-selected");
    if (ariaSelected === "true") state.selected = true;
    const ariaPressed = element.getAttribute("aria-pressed");
    if (ariaPressed === "true" || ariaPressed === "mixed") {
      state.pressed = ariaPressed === "mixed" ? "mixed" : true;
    }
    const ariaChecked = element.getAttribute("aria-checked");
    if (ariaChecked === "true" || ariaChecked === "mixed") {
      state.checked = ariaChecked === "mixed" ? "mixed" : true;
    }
    if (element instanceof HTMLInputElement) {
      if (element.type === "checkbox" || element.type === "radio") {
        if (element.checked) state.checked = true;
      }
    }
    if (element instanceof HTMLOptionElement && element.selected) {
      state.selected = true;
    }
    if (element === document.activeElement) state.focused = true;
    return Object.keys(state).length > 0 ? state : void 0;
  }
  function directTextContent(element) {
    const parts = [];
    for (const node of Array.from(element.childNodes)) {
      if (node.nodeType !== Node.TEXT_NODE) continue;
      const text = normalizeText(node.textContent);
      if (text) parts.push(text);
    }
    return parts.join(" ");
  }
  function normalizeText(text) {
    return (text ?? "").replace(/\s+/g, " ").trim();
  }
  const EVALUATE_VIA_CDP = "evaluateViaCdp";
  const CDP_INPUT_RELAY_ACTIONS = /* @__PURE__ */ new Set([
    "click",
    "dblclick",
    "rightclick",
    "pressKey",
    "keyDown",
    "keyUp",
    "mouseDown",
    "mouseUp",
    "mouseMove",
    "drag",
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
      case "find":
        return handleFind(requireTarget(target, action), options);
      case "findAll":
        return handleFindAll(requireTarget(target, action), options);
      case "collectAll":
        return handleCollectAll(requireTarget(target, action), options);
      case "extractTable":
        return handleExtractTable(requireTarget(target, action));
      case "extractList":
        return handleExtractList(requireTarget(target, action), options);
      case "wait":
        return handleWait(
          requireTarget(target, action),
          options == null ? void 0 : options.timeout,
          (options == null ? void 0 : options.gone) === true,
          options == null ? void 0 : options.interval
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
      case "snapshot":
        return getSnapshot();
      case "xpath":
        return handleXPath(requireTarget(target, action));
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
      case "keyDown":
        return handleKeyDown(
          target,
          requireValue(value, action),
          waitTimeout(options)
        );
      case "keyUp":
        return handleKeyUp(
          target,
          requireValue(value, action),
          waitTimeout(options)
        );
      case "mouseDown":
        return handleMouseDown(
          requireTarget(target, action),
          waitTimeout(options)
        );
      case "mouseUp":
        return handleMouseUp(requireTarget(target, action), waitTimeout(options));
      case "mouseMove":
        return handleMouseMove(
          requireTarget(target, action),
          waitTimeout(options)
        );
      case "drag": {
        const endTarget = (options == null ? void 0 : options.endTarget) ?? (options == null ? void 0 : options.target);
        const endSel = endTarget ? endTarget : value ? { selector: value } : void 0;
        if (!endSel) {
          throw new Error(
            `Action "drag" requires a destination selector (value or options.endTarget)`
          );
        }
        return handleDrag(
          requireTarget(target, action),
          endSel,
          waitTimeout(options),
          (options == null ? void 0 : options.steps) ?? 5,
          (options == null ? void 0 : options.delay) ?? 0
        );
      }
      case "selectText":
        return handleSelectText(
          requireTarget(target, action),
          waitTimeout(options)
        );
      case "uploadFiles":
        return handleUploadFiles(
          requireTarget(target, action),
          options,
          waitTimeout(options)
        );
      case "prepareClick":
        return handlePrepareClick(
          requireTarget(target, action),
          waitTimeout(options)
        );
      case "prepareKeys":
        return handlePrepareKeys(target, waitTimeout(options));
      case "navigate":
        return handleNavigate(requireValue(value, action));
      case "reload":
        return handleReload();
      case "goBack":
        return handleGoBack();
      case "goForward":
        return handleGoForward();
      case "screenshot":
        return handleScreenshot();
      case "evaluate":
        return handleEvaluate(requireValue(value, action));
      case "fetch":
        return handleFetchViaBackground(requireValue(value, action), options);
      case "printToPDF":
        return handlePrintToPDF(options == null ? void 0 : options.tabId);
      case "highlight":
        return handleHighlight(
          requireTarget(target, action),
          waitTimeout(options)
        );
      case "unhighlight":
        return handleUnhighlight();
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
  async function handleCollectAll(target, options) {
    const result = await collectAll(target, {
      maxSteps: typeof (options == null ? void 0 : options.maxSteps) === "number" ? options.maxSteps : void 0,
      stepDelayMs: typeof (options == null ? void 0 : options.stepDelayMs) === "number" ? options.stepDelayMs : void 0,
      maxItems: typeof (options == null ? void 0 : options.maxItems) === "number" ? options.maxItems : void 0,
      visibleOnly: (options == null ? void 0 : options.visibleOnly) === true,
      // Mint each ref at HARVEST time, while the row's node is still live.
      // Minting after the loop gave every row a detached node, or (with a
      // recycling virtualizer) the same node for several different rows.
      onRow: (options == null ? void 0 : options.assignRef) ? (el, info) => {
        info.ref = assignRef(el);
      } : void 0
    });
    return {
      elements: result.elements,
      steps: result.steps,
      truncated: result.truncated,
      truncatedBy: result.truncatedBy,
      scrolled: result.scrolled,
      keyStrategy: result.keyStrategy
    };
  }
  async function handleExtractTable(target) {
    const root = findAllElements(target)[0];
    if (!root) {
      throw new Error(
        `Element not found: ${target.selector ?? target.xpath ?? ""}`
      );
    }
    return extractTable(root);
  }
  async function handleExtractList(target, options) {
    const roots = findAllElements(target);
    if (roots.length === 0) {
      throw new Error(
        `Element not found: ${target.selector ?? target.xpath ?? ""}`
      );
    }
    const fields = (options == null ? void 0 : options.fields) && typeof options.fields === "object" ? options.fields : void 0;
    return extractListFrom(roots, fields, {
      container: (options == null ? void 0 : options.container) === true,
      visibleOnly: (options == null ? void 0 : options.visibleOnly) === true
    });
  }
  function isAbsent(target) {
    try {
      return findAllElements(target).length === 0;
    } catch (error) {
      if (error instanceof DetachedRefError) {
        console.debug(
          "[HTR NControl] wait --gone: ref detached, element is gone",
          error.message
        );
        return true;
      }
      throw error;
    }
  }
  async function handleWait(target, timeout, gone = false, interval) {
    const timeoutMs = timeout ?? 5e3;
    const label = target.selector ?? target.xpath ?? JSON.stringify(target);
    if (gone) {
      if (interval === void 0 || interval <= 0) {
        throw new Error(`wait: --interval must be > 0, got ${interval}`);
      }
      const deadline = Date.now() + timeoutMs;
      for (; ; ) {
        const loading = document.readyState === "loading";
        if (!loading && isAbsent(target)) return { gone: true };
        const remaining = deadline - Date.now();
        if (remaining <= 0) {
          throw new Error(
            loading ? `wait: "${label}" not confirmed gone, document still loading after ${timeoutMs}ms` : `wait: element "${label}" still present after ${timeoutMs}ms`
          );
        }
        await new Promise(
          (resolve) => setTimeout(resolve, Math.min(interval, remaining))
        );
      }
    }
    const element = await waitForElement(target, timeoutMs, {
      force: true,
      throwOnTimeout: true
    });
    if (!element) {
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
    const element = await waitForScrollableActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    element.scrollIntoView({ behavior: "instant", block: "center" });
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
    const element = await waitForScrollableActionableElement(target, {
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
    const element = await waitForScrollableActionableElement(target, {
      timeoutMs,
      requireEnabled: false
    });
    element.scrollIntoView({ behavior: "instant", block: "center" });
    await waitForScrollSettle();
  }
  async function handleFill(target, value, timeoutMs = 5e3) {
    var _a;
    const element = await waitForScrollableActionableElement(target, {
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
    } else if (element instanceof HTMLElement && element.isContentEditable) {
      element.focus();
      selectElementContents(element);
      if (value === "") {
        document.execCommand("delete");
      } else {
        document.execCommand("insertText", false, value);
      }
      element.dispatchEvent(new Event("input", { bubbles: true }));
      element.dispatchEvent(new Event("change", { bubbles: true }));
      element.dispatchEvent(new Event("blur", { bubbles: true }));
    } else {
      throw new Error(
        "Element is not a form input and is not contenteditable — use `type` for a rich-text surface"
      );
    }
  }
  async function handleType(target, value, timeoutMs = 5e3) {
    const element = await waitForScrollableActionableElement(target, {
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
    } else if (element instanceof HTMLElement && element.isContentEditable) {
      element.focus();
      placeCaretAtEnd(element);
      document.execCommand("insertText", false, value);
      element.dispatchEvent(new Event("input", { bubbles: true }));
      element.dispatchEvent(new Event("change", { bubbles: true }));
      element.dispatchEvent(new Event("blur", { bubbles: true }));
    } else {
      throw new Error("Element is not a text input");
    }
  }
  function placeCaretAtEnd(element) {
    const selection = document.getSelection();
    if (!selection) return;
    const range = document.createRange();
    range.selectNodeContents(element);
    range.collapse(false);
    selection.removeAllRanges();
    selection.addRange(range);
  }
  function selectElementContents(element) {
    const selection = document.getSelection();
    if (!selection) return;
    const range = document.createRange();
    range.selectNodeContents(element);
    selection.removeAllRanges();
    selection.addRange(range);
  }
  async function handleClear(target, timeoutMs = 5e3) {
    const element = await waitForScrollableActionableElement(target, {
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
    const element = await waitForScrollableActionableElement(target, {
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
    const element = await waitForScrollableActionableElement(target, {
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
      return;
    }
    if (isAriaCheckbox(element)) {
      await setAriaCheckbox(element, checked);
      return;
    }
    throw new Error("Element is not a checkbox/radio input");
  }
  function isAriaCheckbox(element) {
    var _a;
    return ((_a = element.getAttribute("role")) == null ? void 0 : _a.toLowerCase()) === "checkbox" && element instanceof HTMLElement;
  }
  async function setAriaCheckbox(element, checked) {
    const target = checked ? "true" : "false";
    const readState = () => element.getAttribute("aria-checked") ?? "false";
    if (readState() === target) return;
    const maxClicks = 2;
    for (let attempt = 0; attempt < maxClicks; attempt++) {
      element.focus();
      element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await new Promise((r) => setTimeout(r, 0));
      if (readState() === target) {
        element.dispatchEvent(new Event("change", { bubbles: true }));
        return;
      }
    }
    throw new Error(
      `ARIA checkbox did not reach aria-checked="${target}" after ${maxClicks} clicks (still "${readState()}")`
    );
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
  async function handleKeyDown(target, key, timeoutMs = 5e3) {
    const element = target ? await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    }) : document.activeElement ?? document.body;
    element.focus();
    const descriptor = resolveKey(key);
    element.dispatchEvent(
      new KeyboardEvent("keydown", {
        bubbles: true,
        key: descriptor.key,
        code: descriptor.code,
        keyCode: descriptor.windowsVirtualKeyCode,
        which: descriptor.windowsVirtualKeyCode
      })
    );
  }
  async function handleKeyUp(target, key, timeoutMs = 5e3) {
    const element = target ? await waitForActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    }) : document.activeElement ?? document.body;
    element.focus();
    const descriptor = resolveKey(key);
    element.dispatchEvent(
      new KeyboardEvent("keyup", {
        bubbles: true,
        key: descriptor.key,
        code: descriptor.code,
        keyCode: descriptor.windowsVirtualKeyCode,
        which: descriptor.windowsVirtualKeyCode
      })
    );
  }
  function resolveSyntheticCoordinateTarget(x, y) {
    if (typeof document.elementFromPoint !== "function") {
      throw new Error(
        `Cannot dispatch coordinate input at (${x}, ${y}): document.elementFromPoint is unavailable`
      );
    }
    const element = document.elementFromPoint(x, y);
    if (!element) {
      throw new Error(
        `Cannot dispatch coordinate input at (${x}, ${y}): no element exists at the requested viewport coordinates`
      );
    }
    return element;
  }
  async function handleMouseDown(target, timeoutMs = 5e3) {
    if (typeof target.x === "number" && typeof target.y === "number") {
      const x2 = target.x;
      const y2 = target.y;
      const mouseInit2 = {
        bubbles: true,
        cancelable: true,
        view: window,
        button: 0,
        clientX: x2,
        clientY: y2
      };
      const pointerInit2 = {
        bubbles: true,
        cancelable: true,
        view: window,
        pointerId: 1,
        pointerType: "mouse",
        button: 0,
        buttons: 1,
        clientX: x2,
        clientY: y2
      };
      const targetElement = resolveSyntheticCoordinateTarget(x2, y2);
      targetElement.dispatchEvent(
        typeof PointerEvent !== "undefined" ? new PointerEvent("pointerdown", pointerInit2) : new MouseEvent("pointerdown", mouseInit2)
      );
      targetElement.dispatchEvent(new MouseEvent("mousedown", mouseInit2));
      return;
    }
    const element = await waitForScrollableActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    element.scrollIntoView({ behavior: "instant", block: "center" });
    const rect = element.getBoundingClientRect();
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    const mouseInit = {
      bubbles: true,
      cancelable: true,
      view: window,
      button: 0,
      clientX: x,
      clientY: y
    };
    const pointerInit = {
      bubbles: true,
      cancelable: true,
      view: window,
      pointerId: 1,
      pointerType: "mouse",
      button: 0,
      buttons: 1,
      clientX: x,
      clientY: y
    };
    const el = element;
    el.dispatchEvent(
      typeof PointerEvent !== "undefined" ? new PointerEvent("pointerdown", pointerInit) : new MouseEvent("pointerdown", mouseInit)
    );
    el.dispatchEvent(new MouseEvent("mousedown", mouseInit));
  }
  async function handleMouseUp(target, timeoutMs = 5e3) {
    if (typeof target.x === "number" && typeof target.y === "number") {
      const x2 = target.x;
      const y2 = target.y;
      const mouseInit2 = {
        bubbles: true,
        cancelable: true,
        view: window,
        button: 0,
        clientX: x2,
        clientY: y2
      };
      const pointerInit2 = {
        bubbles: true,
        cancelable: true,
        view: window,
        pointerId: 1,
        pointerType: "mouse",
        button: 0,
        buttons: 0,
        clientX: x2,
        clientY: y2
      };
      const targetElement = resolveSyntheticCoordinateTarget(x2, y2);
      targetElement.dispatchEvent(
        typeof PointerEvent !== "undefined" ? new PointerEvent("pointerup", pointerInit2) : new MouseEvent("pointerup", mouseInit2)
      );
      targetElement.dispatchEvent(new MouseEvent("mouseup", mouseInit2));
      return;
    }
    const element = await waitForScrollableActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    element.scrollIntoView({ behavior: "instant", block: "center" });
    const rect = element.getBoundingClientRect();
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    const mouseInit = {
      bubbles: true,
      cancelable: true,
      view: window,
      button: 0,
      clientX: x,
      clientY: y
    };
    const pointerInit = {
      bubbles: true,
      cancelable: true,
      view: window,
      pointerId: 1,
      pointerType: "mouse",
      button: 0,
      buttons: 0,
      clientX: x,
      clientY: y
    };
    const el = element;
    el.dispatchEvent(
      typeof PointerEvent !== "undefined" ? new PointerEvent("pointerup", pointerInit) : new MouseEvent("pointerup", mouseInit)
    );
    el.dispatchEvent(new MouseEvent("mouseup", mouseInit));
  }
  async function handleMouseMove(target, timeoutMs = 5e3) {
    if (typeof target.x === "number" && typeof target.y === "number") {
      const x2 = target.x;
      const y2 = target.y;
      const mouseInit2 = {
        bubbles: true,
        cancelable: true,
        view: window,
        clientX: x2,
        clientY: y2
      };
      const pointerInit2 = {
        bubbles: true,
        cancelable: true,
        view: window,
        pointerId: 1,
        pointerType: "mouse",
        button: 0,
        buttons: 0,
        clientX: x2,
        clientY: y2
      };
      const targetElement = resolveSyntheticCoordinateTarget(x2, y2);
      targetElement.dispatchEvent(
        typeof PointerEvent !== "undefined" ? new PointerEvent("pointermove", pointerInit2) : new MouseEvent("pointermove", mouseInit2)
      );
      targetElement.dispatchEvent(new MouseEvent("mousemove", mouseInit2));
      return;
    }
    const element = await waitForScrollableActionableElement(target, {
      timeoutMs,
      requireEnabled: false
    });
    element.scrollIntoView({ behavior: "instant", block: "center" });
    const rect = element.getBoundingClientRect();
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    const mouseInit = {
      bubbles: true,
      cancelable: true,
      view: window,
      clientX: x,
      clientY: y
    };
    const pointerInit = {
      bubbles: true,
      cancelable: true,
      view: window,
      pointerId: 1,
      pointerType: "mouse",
      clientX: x,
      clientY: y
    };
    const el = element;
    el.dispatchEvent(
      typeof PointerEvent !== "undefined" ? new PointerEvent("pointermove", pointerInit) : new MouseEvent("pointermove", mouseInit)
    );
    el.dispatchEvent(new MouseEvent("mousemove", mouseInit));
  }
  async function handleDrag(source, dest, timeoutMs = 5e3, steps = 5, delayMs = 0) {
    const pinned = (sel) => typeof sel.x === "number" && typeof sel.y === "number" ? { x: sel.x, y: sel.y } : void 0;
    const srcPin = pinned(source);
    const dstPin = pinned(dest);
    const waitOpts = { timeoutMs, requireEnabled: true };
    const srcWait = srcPin === void 0 ? await waitForScrollableActionableElement(source, waitOpts) : void 0;
    const dstWait = dstPin === void 0 ? await waitForScrollableActionableElement(dest, waitOpts) : void 0;
    if (srcWait !== void 0) {
      srcWait.scrollIntoView({ behavior: "instant", block: "center" });
    }
    if (dstWait !== void 0) {
      dstWait.scrollIntoView({ behavior: "instant", block: "center" });
    }
    const readPoint = (pin, el) => {
      if (el !== void 0) {
        const rect = el.getBoundingClientRect();
        return {
          x: rect.left + rect.width / 2,
          y: rect.top + rect.height / 2,
          element: el
        };
      }
      if (pin === void 0) {
        throw new Error("drag endpoint has neither coordinates nor an element");
      }
      return {
        x: pin.x,
        y: pin.y,
        element: resolveSyntheticCoordinateTarget(pin.x, pin.y)
      };
    };
    const inViewport = (p) => p.x >= 0 && p.y >= 0 && p.x < window.innerWidth && p.y < window.innerHeight;
    const selectorPoints = () => {
      const points = [];
      if (srcWait !== void 0) points.push(readPoint(void 0, srcWait));
      if (dstWait !== void 0) points.push(readPoint(void 0, dstWait));
      return points;
    };
    let src = readPoint(srcPin, srcWait);
    let dst = readPoint(dstPin, dstWait);
    if (selectorPoints().some((p) => !inViewport(p))) {
      const pts = selectorPoints();
      const xs = pts.map((p) => p.x);
      const ys = pts.map((p) => p.y);
      window.scrollBy({
        left: (Math.min(...xs) + Math.max(...xs)) / 2 - window.innerWidth / 2,
        top: (Math.min(...ys) + Math.max(...ys)) / 2 - window.innerHeight / 2,
        behavior: "instant"
      });
      src = readPoint(srcPin, srcWait);
      dst = readPoint(dstPin, dstWait);
      if (selectorPoints().some((p) => !inViewport(p))) {
        throw new Error(
          `drag endpoints (${Math.round(src.x)},${Math.round(src.y)}) and (${Math.round(dst.x)},${Math.round(dst.y)}) are still outside the ${window.innerWidth}x${window.innerHeight} viewport after scrolling; not draggable`
        );
      }
    }
    const sx = src.x;
    const sy = src.y;
    const dx = dst.x;
    const dy = dst.y;
    const srcEl = src.element;
    const dstEl = dst.element;
    const n = Math.max(1, Math.min(100, Math.floor(steps)));
    const downMouse = {
      bubbles: true,
      cancelable: true,
      view: window,
      button: 0,
      clientX: sx,
      clientY: sy
    };
    const downPointer = {
      bubbles: true,
      cancelable: true,
      view: window,
      pointerId: 1,
      pointerType: "mouse",
      button: 0,
      buttons: 1,
      clientX: sx,
      clientY: sy
    };
    const s = srcEl;
    s.dispatchEvent(
      typeof PointerEvent !== "undefined" ? new PointerEvent("pointerdown", downPointer) : new MouseEvent("pointerdown", downMouse)
    );
    s.dispatchEvent(new MouseEvent("mousedown", downMouse));
    for (let i = 1; i <= n; i++) {
      const t = i / n;
      const x = sx + (dx - sx) * t;
      const y = sy + (dy - sy) * t;
      const moveMouse = {
        bubbles: true,
        cancelable: true,
        view: window,
        clientX: x,
        clientY: y
      };
      const movePointer = {
        bubbles: true,
        cancelable: true,
        view: window,
        pointerId: 1,
        pointerType: "mouse",
        buttons: 1,
        clientX: x,
        clientY: y
      };
      const targetEl = i === n ? dstEl : resolveSyntheticCoordinateTarget(x, y);
      targetEl.dispatchEvent(
        typeof PointerEvent !== "undefined" ? new PointerEvent("pointermove", movePointer) : new MouseEvent("pointermove", moveMouse)
      );
      targetEl.dispatchEvent(new MouseEvent("mousemove", moveMouse));
      if (delayMs > 0) {
        await new Promise((r) => setTimeout(r, delayMs));
      }
    }
    const upMouse = {
      bubbles: true,
      cancelable: true,
      view: window,
      button: 0,
      clientX: dx,
      clientY: dy
    };
    const upPointer = {
      bubbles: true,
      cancelable: true,
      view: window,
      pointerId: 1,
      pointerType: "mouse",
      button: 0,
      buttons: 0,
      clientX: dx,
      clientY: dy
    };
    const d = dstEl;
    d.dispatchEvent(
      typeof PointerEvent !== "undefined" ? new PointerEvent("pointerup", upPointer) : new MouseEvent("pointerup", upMouse)
    );
    d.dispatchEvent(new MouseEvent("mouseup", upMouse));
  }
  async function handlePrepareClick(target, timeoutMs = 5e3) {
    const element = await waitForScrollableActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    element.scrollIntoView({ behavior: "instant", block: "center" });
    const rect = element.getBoundingClientRect();
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    return { x, y };
  }
  async function handlePrepareKeys(target, timeoutMs = 5e3) {
    if (!target) {
      return { focused: document.activeElement != null };
    }
    const element = await waitForScrollableActionableElement(target, {
      timeoutMs,
      requireEnabled: true
    });
    element.scrollIntoView({ behavior: "instant", block: "center" });
    element.focus();
    if (element.isContentEditable) placeCaretAtEnd(element);
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
  function decodeBase64(data) {
    if (typeof atob !== "undefined") {
      const bin = atob(data);
      const bytes = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) {
        bytes[i] = bin.charCodeAt(i);
      }
      return bytes;
    }
    return Uint8Array.from(Buffer.from(data, "base64"));
  }
  async function handleUploadFiles(target, options, timeoutMs = 5e3) {
    const filesData = options == null ? void 0 : options.filesData;
    if (!filesData || filesData.length === 0) {
      throw new Error(
        "uploadFiles requires options.filesData (base64 file bytes). On Chrome this action is background-handled via chrome.debugger; this content-script path is used on Firefox."
      );
    }
    let element = findElement(target);
    if (!element) {
      element = await waitForElement(target, timeoutMs, { force: true });
    }
    if (!element) {
      throw new Error(
        `uploadFiles: element ${target.selector ?? target.name ?? "?"} not found within ${timeoutMs}ms`
      );
    }
    if (!(element instanceof HTMLInputElement) || element.type !== "file") {
      throw new Error(
        `uploadFiles target is not an <input type=file> (got ${element instanceof HTMLInputElement ? element.type : element.tagName})`
      );
    }
    if (typeof DataTransfer === "undefined" || typeof File === "undefined") {
      throw new Error(
        "uploadFiles unsupported in this browser (no File/DataTransfer)"
      );
    }
    const dt = new DataTransfer();
    for (const f of filesData) {
      const bytes = decodeBase64(f.base64);
      const buffer = bytes.buffer.slice(
        bytes.byteOffset,
        bytes.byteOffset + bytes.byteLength
      );
      dt.items.add(
        new File([buffer], f.name, {
          type: f.mimeType || "application/octet-stream"
        })
      );
    }
    element.files = dt.files;
    element.dispatchEvent(new Event("input", { bubbles: true }));
    element.dispatchEvent(new Event("change", { bubbles: true }));
    return {
      files: filesData.map((f) => f.name),
      engine: "DataTransfer"
    };
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
