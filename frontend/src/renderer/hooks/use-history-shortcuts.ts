import { useEffect } from "react";
import { isMac } from "@/lib/platform";

type Modifier = "metaKey" | "altKey";
type Shortcut = { code: string; label: string };
type HistoryShortcuts = { modifier: Modifier; back: Shortcut; forward: Shortcut };
type HistoryMoves = { back: () => void; forward: () => void };
type HistoryKey = Pick<KeyboardEvent, "code" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">;

const MAC_SHORTCUTS: HistoryShortcuts = {
  modifier: "metaKey",
  back: { code: "BracketLeft", label: "⌘[" },
  forward: { code: "BracketRight", label: "⌘]" },
};

const OTHER_SHORTCUTS: HistoryShortcuts = {
  modifier: "altKey",
  back: { code: "ArrowLeft", label: "Alt+←" },
  forward: { code: "ArrowRight", label: "Alt+→" },
};

const MOUSE_BACK_BUTTON = 3;
const MOUSE_FORWARD_BUTTON = 4;

export function historyShortcuts(): HistoryShortcuts {
  return isMac ? MAC_SHORTCUTS : OTHER_SHORTCUTS;
}

function onlyModifier(event: HistoryKey, modifier: Modifier): boolean {
  const pressed = [event.metaKey, event.ctrlKey, event.altKey, event.shiftKey].filter(Boolean).length;
  return event[modifier] && pressed === 1;
}

function keyMove(event: HistoryKey, moves: HistoryMoves): (() => void) | undefined {
  const shortcuts = historyShortcuts();
  if (!onlyModifier(event, shortcuts.modifier)) return undefined;
  if (event.code === shortcuts.back.code) return moves.back;
  if (event.code === shortcuts.forward.code) return moves.forward;
  return undefined;
}

function mouseMove(button: number, moves: HistoryMoves): (() => void) | undefined {
  if (button === MOUSE_BACK_BUTTON) return moves.back;
  if (button === MOUSE_FORWARD_BUTTON) return moves.forward;
  return undefined;
}

function typesText(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement;
}

export function useHistoryShortcuts(back: () => void, forward: () => void) {
  useEffect(() => {
    const moves = { back, forward };

    function run(event: Event, move: (() => void) | undefined) {
      if (!move) return;
      event.preventDefault();
      move();
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.defaultPrevented || typesText(event.target)) return;
      run(event, keyMove(event, moves));
    }

    function onMouseUp(event: MouseEvent) {
      if (event.defaultPrevented) return;
      run(event, mouseMove(event.button, moves));
    }

    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("mouseup", onMouseUp);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("mouseup", onMouseUp);
    };
  }, [back, forward]);
}
