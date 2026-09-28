import { useEffect } from "react";
import { isMac } from "@/lib/platform";

export type HistoryMove = "back" | "forward";

const MOUSE_BACK_BUTTON = 3;
const MOUSE_FORWARD_BUTTON = 4;

type HistoryKey = Pick<KeyboardEvent, "code" | "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">;

function onlyModifier(event: HistoryKey, modifier: "metaKey" | "altKey"): boolean {
  const pressed = [event.metaKey, event.ctrlKey, event.altKey, event.shiftKey].filter(Boolean).length;
  return event[modifier] && pressed === 1;
}

function macShortcut(event: HistoryKey): HistoryMove | null {
  if (!onlyModifier(event, "metaKey")) return null;
  if (event.code === "BracketLeft") return "back";
  if (event.code === "BracketRight") return "forward";
  return null;
}

function altShortcut(event: HistoryKey): HistoryMove | null {
  if (!onlyModifier(event, "altKey")) return null;
  if (event.key === "ArrowLeft") return "back";
  if (event.key === "ArrowRight") return "forward";
  return null;
}

export function historyShortcut(event: HistoryKey, mac: boolean): HistoryMove | null {
  return mac ? macShortcut(event) : altShortcut(event);
}

export function historyMouseButton(button: number): HistoryMove | null {
  if (button === MOUSE_BACK_BUTTON) return "back";
  if (button === MOUSE_FORWARD_BUTTON) return "forward";
  return null;
}

function typesText(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement;
}

export function useHistoryShortcuts(back: () => void, forward: () => void) {
  useEffect(() => {
    const moves: Record<HistoryMove, () => void> = { back, forward };

    function run(event: Event, move: HistoryMove | null) {
      if (!move) return;
      event.preventDefault();
      moves[move]();
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.defaultPrevented || typesText(event.target)) return;
      run(event, historyShortcut(event, isMac));
    }

    function onMouseUp(event: MouseEvent) {
      if (event.defaultPrevented) return;
      run(event, historyMouseButton(event.button));
    }

    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("mouseup", onMouseUp);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("mouseup", onMouseUp);
    };
  }, [back, forward]);
}
