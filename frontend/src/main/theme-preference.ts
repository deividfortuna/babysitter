import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { isThemePreference, type ThemePreference } from "../shared/theme";

const FILE = "theme.json";

export function readStoredTheme(dir: string): ThemePreference {
  try {
    const raw = JSON.parse(readFileSync(path.join(dir, FILE), "utf8")) as { theme?: unknown };
    return isThemePreference(raw.theme) ? raw.theme : "system";
  } catch {
    return "system";
  }
}

export function writeStoredTheme(dir: string, preference: ThemePreference): void {
  try {
    mkdirSync(dir, { recursive: true });
    writeFileSync(path.join(dir, FILE), JSON.stringify({ theme: preference }));
  } catch {}
}
