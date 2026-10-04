export function relativeTime(iso: string, now: number = Date.now()): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const seconds = Math.round((now - then) / 1000);
  if (seconds < 45) return "just now";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 36) return `${hours} h ago`;
  const days = Math.round(hours / 24);
  return `${days} d ago`;
}

export function duration(fromIso: string, toIso?: string | null, now: number = Date.now()): string {
  const from = new Date(fromIso).getTime();
  const to = toIso ? new Date(toIso).getTime() : now;
  if (Number.isNaN(from) || Number.isNaN(to)) return "";
  const minutes = Math.max(0, Math.round((to - from) / 60_000));
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h ${minutes % 60}m`;
  const days = Math.floor(hours / 24);
  return `${days}d ${hours % 24}h`;
}

export function shortSha(sha: string): string {
  return sha.length > 7 ? sha.slice(0, 7) : sha;
}

export function shortDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function dayOf(iso: string | null | undefined, today: string): string {
  const isToday = iso ? new Date(iso).toDateString() === today : false;
  return isToday ? "Today" : "Earlier";
}

export function groupByDay<T>(items: T[], dateOf: (item: T) => string | null | undefined): [string, T[]][] {
  const today = new Date().toDateString();
  const days = new Map<string, T[]>([
    ["Today", []],
    ["Earlier", []],
  ]);
  for (const item of items) days.get(dayOf(dateOf(item), today))?.push(item);
  return [...days].filter(([, list]) => list.length > 0);
}
