export type MenuAnchor = { x: number; y: number };

export function isMenuAnchor(value: unknown): value is MenuAnchor {
  if (typeof value !== "object" || value === null) return false;
  const { x, y } = value as Record<string, unknown>;
  return Number.isFinite(x) && Number.isFinite(y);
}
