import { useCallback, useState } from "react";

const STORAGE_KEY = "diff_preferences";

export type DiffStyle = "unified" | "split";

export type DiffPreferences = { style: DiffStyle; wrap: boolean };

const DEFAULTS: DiffPreferences = { style: "unified", wrap: false };

function isStyle(value: unknown): value is DiffStyle {
  return value === "unified" || value === "split";
}

function readStored(): DiffPreferences {
  try {
    const stored: unknown = JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "null");
    if (typeof stored !== "object" || stored === null) return DEFAULTS;
    const { style, wrap } = stored as Record<string, unknown>;
    return { style: isStyle(style) ? style : DEFAULTS.style, wrap: wrap === true };
  } catch {
    return DEFAULTS;
  }
}

function store(preferences: DiffPreferences): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(preferences));
  } catch {}
}

export function useDiffPreferences() {
  const [preferences, setPreferences] = useState(readStored);

  const change = useCallback((next: Partial<DiffPreferences>) => {
    setPreferences((current) => {
      const updated = { ...current, ...next };
      store(updated);
      return updated;
    });
  }, []);

  return { preferences, change };
}
