import { useEffect, useRef } from "react";
import { anyDialogOpen, pressedKey } from "@/hooks/use-command-shortcut";
import { onlyModifiers } from "@/lib/shortcuts";

const TYPING_CONTEXT_SELECTOR = [
  "input",
  "textarea",
  "select",
  '[contenteditable]:not([contenteditable="false"])',
  '[role="textbox"]',
  '[role="combobox"]',
  '[role="listbox"]',
  '[role="menu"]',
].join(",");

function inTypingContext(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(TYPING_CONTEXT_SELECTOR) !== null;
}

export function pressesPlainKey(event: KeyboardEvent, key: string): boolean {
  if (event.isComposing || event.repeat) return false;
  return pressedKey(event) === key && onlyModifiers(event, []);
}

function runs(event: KeyboardEvent, key: string): boolean {
  if (event.defaultPrevented || inTypingContext(event.target) || anyDialogOpen()) return false;
  return pressesPlainKey(event, key);
}

export function usePlainShortcut(key: string, onPress: () => void, allowed: boolean) {
  const press = useRef(onPress);
  useEffect(() => {
    press.current = onPress;
  });

  useEffect(() => {
    if (!allowed) return;

    function onKeyDown(event: KeyboardEvent) {
      if (!runs(event, key)) return;
      event.preventDefault();
      press.current();
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [key, allowed]);
}
