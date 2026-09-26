import { CANVAS, isThemePreference, type ThemePreference } from "../../shared/theme";

export type { ThemePreference };
export type Theme = "light" | "dark";

const THEME_STORAGE_KEY = "theme";
const DARK_SCHEME = "(prefers-color-scheme: dark)";

export function readThemePreference(): ThemePreference {
  try {
    const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
    return isThemePreference(stored) ? stored : "system";
  } catch {
    return "system";
  }
}

export function storeThemePreference(preference: ThemePreference): void {
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, preference);
  } catch {}
}

export function systemTheme(): Theme {
  return window.matchMedia?.(DARK_SCHEME).matches ? "dark" : "light";
}

export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle("dark", theme === "dark");
  document.documentElement.style.backgroundColor = CANVAS[theme];
}

export function watchSystemTheme(onChange: () => void): () => void {
  const query = window.matchMedia?.(DARK_SCHEME);
  if (!query) return () => {};
  query.addEventListener("change", onChange);
  return () => query.removeEventListener("change", onChange);
}
