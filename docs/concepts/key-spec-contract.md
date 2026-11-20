---
type: Concept
title: Key-spec contract — one grammar for `press`/`keydown`/`keyup` on both transports
description: The shared keyboard-spec grammar (modifier aliases, the `+` key special case, Shift derivation, text suppression, Enter as CR, loud rejection of unknown names) implemented twice — src/utils/keyMap.ts for the extension and htrcli/internal/cdp/input.go for --cdp — and the bundle check that keeps them from drifting.
resource: src/utils/keyMap.ts; htrcli/internal/cdp/input.go; src/contentScript/commandExecutor.ts; scripts/check-cdp-bundle.ts
tags: [keyboard, input, press, cdp, extension, contract]
timestamp: 2026-11-19T03:54:00Z
status: active
---

# Key-spec contract

The `press` verb, `keydown`/`keyup`, and the extension's `pressKey` action take one string that names a modifier combo plus a key: `"Enter"`, `"a"`, `"Ctrl+a"`, `"Ctrl+Shift+a"`, `"Meta+z"`, `"F5"`, `"+"`, `"Ctrl++"`. The same grammar and the same emitted payload must hold on both transports:

- Extension: `src/utils/keyMap.ts` (`resolveKey`, `resolveKeySpec`) consumed by `src/contentScript/commandExecutor.ts`.
- `--cdp`: `parseKey`/`keyParams` in `htrcli/internal/cdp/input.go`.

## Grammar

- Split on `+`. The last segment is the key, the preceding segments are modifiers.
- **`+` is both separator and key.** A bare `"+"` is the plus key; `"Ctrl++"` is Ctrl held with it. A lone trailing `"+"` after a modifier (`"Ctrl+"`) names no key and stays an error.
- Whitespace around any segment is trimmed (`"Ctrl + a"` works).
- Modifier aliases are case-insensitive: `Ctrl`/`Control`; `Alt`/`Option`; `Meta`/`Cmd`/`Command`; `Shift`.
- A bare modifier (`"Ctrl"`, `"Cmd"`, `"Shift"`) resolves as a key so `keydown`/`keyup` can hold it.
- An unknown multi-character name (`"Ener"`) is an error, never a dispatched key: the caller gets a clear failure instead of exit 0 with garbage input.

## Shift semantics

- `Ctrl+Shift+a` produces key `"A"`, not `"a"`; the Shift transform applies regardless of any other modifier.
- `Shift+1` is `"!"`, `Shift+-` is `"_"`: digits and punctuation map through the shifted-character table, because `toUpperCase` on a digit is a no-op.
- A character that only exists with Shift (`"A"`, `"!"`) derives the Shift bit when the spec supplied no modifier, so `key` and `shiftKey` agree instead of claiming a shifted character with `shiftKey: false`.
- Ctrl/Alt/Meta combinations insert no text for a single character (the browser produces none). Named keys keep their text: **`Ctrl+Enter` still sends `"\r"`** on both transports, matching Go's `keyParams`, where only single characters are `printable`.
- `Enter` is `key: "Enter"`, `code: "Enter"`, VK 13, `text: "\r"`; letters use `KeyX` with a shift-less VK; digits use `DigitN`; symbols map to their physical US-QWERTY code.

## Descriptor table

Both implementations model the US QWERTY layout: named keys (`Enter`, `Tab`, `Escape`, `Backspace`, `Delete`, arrows, `Home`, `End`, `PageUp`/`PageDown`, `F1`–`F12`, bare modifiers), letters, digits, unshifted symbols, and shifted symbols (which share the base key's code and VK). `resolveKey` on its own only accepts named keys and single printable ASCII characters; combos go through `resolveKeySpec`.

## Keeping them in sync

- The `--cdp` path runs a DOM bundle built from `src/` and committed as `htrcli/internal/cdp/bundle/htrcli-dom.js` (`//go:embed`). Editing it by hand, or changing `src/contentScript`/`src/utils` without rebuilding, makes the two transports disagree.
- `bun run cdp-bundle:check` rebuilds into a temp dir and byte-compares against the committed file; `bun run build` runs `cdp-bundle:build` so a normal build refreshes it. Run the check whenever key handling changes.

## Tests

- `src/utils/keyMap.test.ts` — descriptor table, `+` handling, whitespace trimming, aliases, Shift+digit/symbol, named keys in combos, F1–F12, bare modifiers, unknown key/modifier rejection, and Shift derivation for `"A"`/`"!"`.
- `htrcli/internal/cdp/input_test.go` — `TestPressEnterKeyParams`, `TestPressCarriesKeyCodeAndSuppressesShortcutText`, `TestKeyDownAndKeyUpUseExactCDPPayloads`, `TestKeyParamsDerivesShiftForShiftedCharacters`, `TestParseKeyAcceptsModifierAliases`, `TestParseKeyAcceptsPlusKey`, `TestKeyInputRejectsInvalidSpecs`.
- `src/background/cdpInput.test.ts` — the extension CDP input path forwards the resolved payload.
