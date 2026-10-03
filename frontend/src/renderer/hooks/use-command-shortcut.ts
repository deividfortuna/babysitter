import { useEffect } from "react";
import { isMac } from "@/lib/platform";
import { commandModifier, onlyModifiers, type Modifier } from "@/lib/shortcuts";

const DIALOG_SELECTOR = '[role="dialog"], [role="alertdialog"]';
const LATIN_LETTER = /^[a-z]$/;
const LATIN_LETTER_OR_DIGIT = /^[a-z0-9]$/i;
const LETTER_CODE = /^Key([A-Z])$/;
const SYMBOL_CODES: Record<string, string> = { Backquote: "`" };

type Options = { alt?: boolean; ctrl?: boolean };

type Chord = Required<Options> & { key: string };

export function anyDialogOpen(): boolean {
  return document.querySelector(DIALOG_SELECTOR) !== null;
}

function chordModifiers({ alt, ctrl }: Chord): Modifier[] {
  const base = ctrl ? "ctrlKey" : commandModifier();
  return alt ? [base, "altKey"] : [base];
}

function keyAtPosition(code: string): string | undefined {
  return code.match(LETTER_CODE)?.[1]?.toLowerCase() ?? SYMBOL_CODES[code];
}

export function pressedKey(event: KeyboardEvent): string {
  const layoutKey = event.key.toLowerCase();
  if (LATIN_LETTER.test(layoutKey)) return layoutKey;
  return keyAtPosition(event.code) ?? layoutKey;
}

function typesAltGraphSymbol(event: KeyboardEvent): boolean {
  return !isMac && event.getModifierState("AltGraph") && !LATIN_LETTER_OR_DIGIT.test(event.key);
}

function presses(event: KeyboardEvent, chord: Chord): boolean {
  if (typesAltGraphSymbol(event)) return false;
  return pressedKey(event) === chord.key && onlyModifiers(event, chordModifiers(chord));
}

function runs(event: KeyboardEvent, chord: Chord): boolean {
  return !event.defaultPrevented && presses(event, chord) && !anyDialogOpen();
}

export function useCommandShortcut(
  key: string,
  onPress: () => void,
  allowed: boolean,
  { alt = false, ctrl = false }: Options = {},
) {
  useEffect(() => {
    if (!allowed) return;
    const chord = { key, alt, ctrl };

    function onKeyDown(event: KeyboardEvent) {
      if (!runs(event, chord)) return;
      event.preventDefault();
      event.stopPropagation();
      if (event.repeat) return;
      onPress();
    }

    window.addEventListener("keydown", onKeyDown, { capture: true });
    return () => window.removeEventListener("keydown", onKeyDown, { capture: true });
  }, [key, onPress, allowed, alt, ctrl]);
}
