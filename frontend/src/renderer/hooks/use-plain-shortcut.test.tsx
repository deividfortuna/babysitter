import { fireEvent, renderHook } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { usePlainShortcut } from "./use-plain-shortcut";

afterEach(() => {
  document.body.replaceChildren();
});

function listenForD(allowed = true) {
  const onPress = vi.fn();
  const view = renderHook(({ allowed }) => usePlainShortcut("d", onPress, allowed), { initialProps: { allowed } });
  return { onPress, ...view };
}

function append<T extends HTMLElement>(element: T): T {
  document.body.append(element);
  return element;
}

test("the key runs the action in either case", () => {
  const { onPress } = listenForD();

  fireEvent.keyDown(window, { key: "d", code: "KeyD" });
  fireEvent.keyDown(window, { key: "D", code: "KeyD" });

  expect(onPress).toHaveBeenCalledTimes(2);
});

test("a key that is not allowed does nothing", () => {
  const { onPress } = listenForD(false);

  fireEvent.keyDown(window, { key: "d", code: "KeyD" });

  expect(onPress).not.toHaveBeenCalled();
});

test("the key stops running when it is no longer allowed", () => {
  const { onPress, rerender } = listenForD();

  rerender({ allowed: false });
  fireEvent.keyDown(window, { key: "d", code: "KeyD" });

  expect(onPress).not.toHaveBeenCalled();
});

test("the latest action runs without a new listener", () => {
  const first = vi.fn();
  const second = vi.fn();
  const { rerender } = renderHook(({ onPress }) => usePlainShortcut("d", onPress, true), {
    initialProps: { onPress: first },
  });

  rerender({ onPress: second });
  fireEvent.keyDown(window, { key: "d", code: "KeyD" });

  expect(first).not.toHaveBeenCalled();
  expect(second).toHaveBeenCalledTimes(1);
});

test.each([
  { name: "a meta key", init: { metaKey: true } },
  { name: "a ctrl key", init: { ctrlKey: true } },
  { name: "an alt key", init: { altKey: true } },
  { name: "a shift key", init: { shiftKey: true } },
  { name: "an IME composition", init: { isComposing: true } },
  { name: "a held key", init: { repeat: true } },
])("$name does not run the action", ({ init }) => {
  const { onPress } = listenForD();

  fireEvent.keyDown(window, { key: "d", code: "KeyD", ...init });

  expect(onPress).not.toHaveBeenCalled();
});

test("another key does not run the action", () => {
  const { onPress } = listenForD();

  fireEvent.keyDown(window, { key: "r", code: "KeyR" });

  expect(onPress).not.toHaveBeenCalled();
});

test.each([
  { name: "an input", make: () => document.createElement("input") },
  { name: "a textarea", make: () => document.createElement("textarea") },
  { name: "a select", make: () => document.createElement("select") },
  {
    name: "a contenteditable element",
    make: () => {
      const element = document.createElement("div");
      element.setAttribute("contenteditable", "true");
      return element;
    },
  },
  {
    name: "a plaintext-only element",
    make: () => {
      const element = document.createElement("div");
      element.setAttribute("contenteditable", "plaintext-only");
      return element;
    },
  },
  {
    name: "a text box role",
    make: () => {
      const element = document.createElement("div");
      element.setAttribute("role", "textbox");
      return element;
    },
  },
  {
    name: "an open list",
    make: () => {
      const element = document.createElement("div");
      element.setAttribute("role", "listbox");
      return element;
    },
  },
  {
    name: "an open menu",
    make: () => {
      const element = document.createElement("div");
      element.setAttribute("role", "menu");
      return element;
    },
  },
])("a key typed in $name does not run the action", ({ make }) => {
  const { onPress } = listenForD();
  const target = append(make());

  fireEvent.keyDown(target, { key: "d", code: "KeyD" });

  expect(onPress).not.toHaveBeenCalled();
});

test("a key typed in a button runs the action", () => {
  const { onPress } = listenForD();
  const button = append(document.createElement("button"));

  fireEvent.keyDown(button, { key: "d", code: "KeyD" });

  expect(onPress).toHaveBeenCalledTimes(1);
});

test("a key does not run the action while a dialog is open", () => {
  const { onPress } = listenForD();
  const dialog = append(document.createElement("div"));
  dialog.setAttribute("role", "dialog");

  fireEvent.keyDown(window, { key: "d", code: "KeyD" });

  expect(onPress).not.toHaveBeenCalled();
});

test("an event that another handler took does not run the action", () => {
  const { onPress } = listenForD();
  const taken = new KeyboardEvent("keydown", { key: "d", code: "KeyD", cancelable: true });
  taken.preventDefault();

  window.dispatchEvent(taken);

  expect(onPress).not.toHaveBeenCalled();
});
