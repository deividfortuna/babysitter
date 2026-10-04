import { expect, test } from "vite-plus/test";
import { pressesPlainKey } from "./use-plain-shortcut";

function keyDown(init: KeyboardEventInit): KeyboardEvent {
  return new KeyboardEvent("keydown", init);
}

test("the key matches in either case", () => {
  expect(pressesPlainKey(keyDown({ key: "d", code: "KeyD" }), "d")).toBe(true);
  expect(pressesPlainKey(keyDown({ key: "D", code: "KeyD" }), "d")).toBe(true);
});

test.each([
  { name: "a meta key", init: { metaKey: true } },
  { name: "a ctrl key", init: { ctrlKey: true } },
  { name: "an alt key", init: { altKey: true } },
  { name: "a shift key", init: { shiftKey: true } },
  { name: "an IME composition", init: { isComposing: true } },
  { name: "a held key", init: { repeat: true } },
])("$name does not match", ({ init }) => {
  expect(pressesPlainKey(keyDown({ key: "d", code: "KeyD", ...init }), "d")).toBe(false);
});

test("another key does not match", () => {
  expect(pressesPlainKey(keyDown({ key: "r", code: "KeyR" }), "d")).toBe(false);
});
