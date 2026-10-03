import { useEffect } from "react";
import { commandModifier, onlyModifier } from "@/lib/shortcuts";

const DIALOG_SELECTOR = '[role="dialog"], [role="alertdialog"]';

function anyDialogOpen(): boolean {
  return document.querySelector(DIALOG_SELECTOR) !== null;
}

function pressesPalette(event: KeyboardEvent): boolean {
  return event.key.toLowerCase() === "p" && onlyModifier(event, commandModifier());
}

function opensPalette(event: KeyboardEvent): boolean {
  return !event.defaultPrevented && pressesPalette(event) && !anyDialogOpen();
}

export function useCommandPaletteShortcut(onOpen: () => void, allowed: boolean) {
  useEffect(() => {
    if (!allowed) return;

    function onKeyDown(event: KeyboardEvent) {
      if (!opensPalette(event)) return;
      event.preventDefault();
      onOpen();
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onOpen, allowed]);
}
