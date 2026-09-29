import type { PullRequest } from "@/hooks/usePulls";
import type { Watch } from "@/hooks/useWatches";
import { isAgentLive, isTakenOver, needsAttention, watchAuthor, watchLabel } from "@/lib/watch-status";

export type WatchFilter = "all" | "needs-you" | "working" | "ready" | "with-you";

type FilterOption = { label: string; matches: (w: Watch) => boolean };

export const watchFilters: Record<WatchFilter, FilterOption> = {
  all: { label: "All", matches: () => true },
  "needs-you": { label: "Needs you", matches: needsAttention },
  working: { label: "Agent working", matches: (w) => isAgentLive(w.session.state) },
  ready: { label: "Ready to merge", matches: (w) => Boolean(w.readySince) },
  "with-you": { label: "With you", matches: isTakenOver },
};

export const watchFilterValues: WatchFilter[] = ["all", "needs-you", "working", "ready", "with-you"];

export function isWatchFilter(value: string): value is WatchFilter {
  return Object.hasOwn(watchFilters, value);
}

export function countByFilter(watches: Watch[]): Map<WatchFilter, number> {
  return new Map(watchFilterValues.map((value) => [value, watches.filter(watchFilters[value].matches).length]));
}

export function searchNeedle(query: string): string {
  return query.trim().toLowerCase();
}

export function watchSearchText(
  w: Pick<Watch, "title" | "repo" | "number" | "author">,
  pull?: Pick<PullRequest, "author" | "labels">,
): string {
  return [w.title, watchLabel(w), watchAuthor(w, pull), ...(pull?.labels ?? [])].join(" ").toLowerCase();
}
