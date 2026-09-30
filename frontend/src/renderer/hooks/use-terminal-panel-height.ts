import { useCallback, useState } from "react";

const STORAGE_KEY = "terminal_panel_height";
export const TERMINAL_PANEL_DEFAULT_HEIGHT = 420;
export const TERMINAL_PANEL_MIN_HEIGHT = 180;
export const TERMINAL_PANEL_MAX_HEIGHT = 1200;

export function clampTerminalPanelHeight(height: number): number {
  return Math.min(TERMINAL_PANEL_MAX_HEIGHT, Math.max(TERMINAL_PANEL_MIN_HEIGHT, Math.round(height)));
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
  const [height, setHeightState] = useState(readStoredHeight);

  const setHeight = useCallback((next: number) => {
    const clamped = clampTerminalPanelHeight(next);
    setHeightState(clamped);
    storeHeight(clamped);
  }, []);

  const resetHeight = useCallback(() => setHeight(TERMINAL_PANEL_DEFAULT_HEIGHT), [setHeight]);

  return { height, setHeight, resetHeight };
}
