export type View =
  | { kind: "watching"; repo?: string }
  | { kind: "repo"; name: string }
  | { kind: "stopped" }
  | { kind: "notifications" }
  | { kind: "watch"; id: number };

export type Navigate = (view: View) => void;

export function sameView(a: View, b: View): boolean {
  if (a.kind !== b.kind) return false;
  if (a.kind === "watching" && b.kind === "watching") return (a.repo ?? "") === (b.repo ?? "");
  if (a.kind === "repo" && b.kind === "repo") return a.name === b.name;
  if ("id" in a && "id" in b) return a.id === b.id;
  return true;
}
