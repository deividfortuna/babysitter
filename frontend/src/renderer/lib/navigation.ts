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

export const HISTORY_LIMIT = 20;

type ViewHistory = { back: View[]; current: View; forward: View[] };

export function startHistory(view: View): ViewHistory {
  return { back: [], current: view, forward: [] };
}

function pushBack(back: View[], view: View): View[] {
  return [...back, view].slice(-HISTORY_LIMIT);
}

export function visit(history: ViewHistory, next: View): ViewHistory {
  if (sameView(history.current, next)) return history;
  return { back: pushBack(history.back, history.current), current: next, forward: [] };
}

export function goBack(history: ViewHistory): ViewHistory {
  const previous = history.back.at(-1);
  if (!previous) return history;
  return { back: history.back.slice(0, -1), current: previous, forward: [history.current, ...history.forward] };
}

export function goForward(history: ViewHistory): ViewHistory {
  const [next, ...rest] = history.forward;
  if (!next) return history;
  return { back: pushBack(history.back, history.current), current: next, forward: rest };
}
