export type QuitShortcutHint = { state: "down" } | { state: "up" };

export function isQuitShortcutHint(value: unknown): value is QuitShortcutHint {
  if (typeof value !== "object" || value === null || !("state" in value)) return false;
  return value.state === "down" || value.state === "up";
}
