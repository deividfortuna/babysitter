import { useEffect } from "react";
import { isMac } from "@/lib/platform";
import { commandModifier, onlyModifiers, type Modifier } from "@/lib/shortcuts";

const DIALOG_SELECTOR = '[role="dialog"], [role="alertdialog"]';
const LATIN_LETTER = /^[a-z]$/;
const LATIN_LETTER_OR_DIGIT = /^[a-z0-9]$/i;
const LETTER_CODE = /^Key([A-Z])$/;

type Options = { alt?: boolean };

function anyDialogOpen(): boolean {
  return document.querySelector(DIALOG_SELECTOR) !== null;
}

function chordModifiers(alt: boolean): Modifier[] {
  return alt ? [commandModifier(), "altKey"] : [commandModifier()];
}

function pressedKey(event: KeyboardEvent): string {
  const layoutKey = event.key.toLowerCase();
  if (LATIN_LETTER.test(layoutKey)) return layoutKey;
  return event.code.match(LETTER_CODE)?.[1]?.toLowerCase() ?? layoutKey;
}

function typesAltGraphSymbol(event: KeyboardEvent): boolean {
  return !isMac && event.getModifierState("AltGraph") && !LATIN_LETTER_OR_DIGIT.test(event.key);
}

function presses(event: KeyboardEvent, key: string, alt: boolean): boolean {
  if (typesAltGraphSymbol(event)) return false;
  return pressedKey(event) === key && onlyModifiers(event, chordModifiers(alt));
}

function runs(event: KeyboardEvent, key: string, alt: boolean): boolean {
  return !event.defaultPrevented && presses(event, key, alt) && !anyDialogOpen();
}

export function useCommandShortcut(key: string, onPress: () => void, allowed: boolean, { alt = false }: Options = {}) {
  useEffect(() => {
    if (!allowed) return;

    function onKeyDown(event: KeyboardEvent) {
      if (!runs(event, key, alt)) return;
      event.preventDefault();
      event.stopPropagation();
      if (event.repeat) return;
      onPress();
    }

    window.addEventListener("keydown", onKeyDown, { capture: true });
    return () => window.removeEventListener("keydown", onKeyDown, { capture: true });
  }, [key, onPress, allowed, alt]);
}
