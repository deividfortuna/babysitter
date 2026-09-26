export type ThemePreference = "system" | "light" | "dark";

export function isThemePreference(value: unknown): value is ThemePreference {
  return value === "system" || value === "light" || value === "dark";
}

export const CANVAS = { light: "#ffffff", dark: "#0d1117" };

export function canvasColor(preference: ThemePreference, systemDark: boolean): string {
  const dark = preference === "dark" || (preference === "system" && systemDark);
  return dark ? CANVAS.dark : CANVAS.light;
}
