import { useEffect } from "react";
import { commandModifier, onlyModifier } from "@/lib/shortcuts";

const DIALOG_SELECTOR = '[role="dialog"], [role="alertdialog"]';

function anyDialogOpen(): boolean {
  return document.querySelector(DIALOG_SELECTOR) !== null;
}

function presses(event: KeyboardEvent, key: string): boolean {
  return event.key.toLowerCase() === key && onlyModifier(event, commandModifier());
}

function runs(event: KeyboardEvent, key: string): boolean {
  return !event.defaultPrevented && presses(event, key) && !anyDialogOpen();
}

export function useCommandShortcut(key: string, onPress: () => void, allowed: boolean) {
  useEffect(() => {
    if (!allowed) return;

    function onKeyDown(event: KeyboardEvent) {
      if (!runs(event, key)) return;
      event.preventDefault();
      if (event.repeat) return;
      onPress();
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [key, onPress, allowed]);
}
