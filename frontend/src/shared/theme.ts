export type ThemePreference = "system" | "light" | "dark";

export function isThemePreference(value: unknown): value is ThemePreference {
  return value === "system" || value === "light" || value === "dark";
}

export const CANVAS = { light: "#fcfcfc", dark: "#0a0a0a" };

export const INK = { light: "#232323", dark: "#f5f5f5" };

function isDark(preference: ThemePreference, systemDark: boolean): boolean {
  return preference === "dark" || (preference === "system" && systemDark);
}

export function canvasColor(preference: ThemePreference, systemDark: boolean): string {
  return isDark(preference, systemDark) ? CANVAS.dark : CANVAS.light;
}

export function windowControlsColors(
  preference: ThemePreference,
  systemDark: boolean,
): { color: string; symbolColor: string } {
  const scheme = isDark(preference, systemDark) ? "dark" : "light";
  return { color: CANVAS[scheme], symbolColor: INK[scheme] };
}
