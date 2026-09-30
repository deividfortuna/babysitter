import { useCallback, useState, useSyncExternalStore } from "react";

const STORAGE_KEY = "terminal_panel_height";
const WINDOW_SHARE = 0.8;
export const TERMINAL_PANEL_DEFAULT_HEIGHT = 420;
export const TERMINAL_PANEL_MIN_HEIGHT = 180;
export const TERMINAL_PANEL_MAX_HEIGHT = 1200;

export function clampTerminalPanelHeight(height: number, maxHeight = TERMINAL_PANEL_MAX_HEIGHT): number {
  return Math.min(maxHeight, Math.max(TERMINAL_PANEL_MIN_HEIGHT, Math.round(height)));
}

function subscribeToWindowSize(onChange: () => void) {
  window.addEventListener("resize", onChange);
  return () => window.removeEventListener("resize", onChange);
}

function windowMaxHeight(): number {
  return clampTerminalPanelHeight(Math.floor(window.innerHeight * WINDOW_SHARE));
}

function readStoredHeight(): number {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    const parsed = raw ? Number(raw) : NaN;
    return Number.isFinite(parsed) ? clampTerminalPanelHeight(parsed) : TERMINAL_PANEL_DEFAULT_HEIGHT;
  } catch {
    return TERMINAL_PANEL_DEFAULT_HEIGHT;
  }
}

function storeHeight(height: number): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, String(height));
  } catch {}
}

export function useTerminalPanelHeight() {
  const [preferredHeight, setPreferredHeight] = useState(readStoredHeight);
  const maxHeight = useSyncExternalStore(subscribeToWindowSize, windowMaxHeight, () => TERMINAL_PANEL_MAX_HEIGHT);
  const height = Math.min(preferredHeight, maxHeight);

  const setHeight = useCallback(
    (next: number) => {
      const clamped = clampTerminalPanelHeight(next, maxHeight);
      setPreferredHeight(clamped);
      storeHeight(clamped);
    },
    [maxHeight],
  );

  const resetHeight = useCallback(() => setHeight(TERMINAL_PANEL_DEFAULT_HEIGHT), [setHeight]);

  return { height, maxHeight, setHeight, resetHeight };
}
