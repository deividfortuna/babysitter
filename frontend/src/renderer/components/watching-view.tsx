import { useMemo, useState } from "react";
import { CircleAlertIcon, SearchIcon } from "lucide-react";
import { usePullsByLabel } from "@/hooks/usePulls";
import { useWatches, type Watch } from "@/hooks/useWatches";
import { FirstRun } from "@/components/first-run";
import { SearchInput } from "@/components/search-input";
import { InboxGroup, PullsErrorAlert } from "@/components/inbox-row";
import { Meta } from "@/components/status-badges";
import { ViewHeader } from "@/components/view-header";
import { WatchRow } from "@/components/watch-row";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import type { Navigate } from "@/lib/navigation";
import {
  countByFilter,
  isWatchFilter,
  searchNeedle,
  watchFilterValues,
  watchFilters,
  watchSearchText,
  type WatchFilter,
} from "@/lib/watch-filter";
import { needsAttention, needsYouFirst, watchLabel } from "@/lib/watch-status";

function byRepo(watches: Watch[]): [string, Watch[]][] {
  const map = new Map<string, Watch[]>();
  for (const w of watches) {
    const list = map.get(w.repo) ?? [];
    list.push(w);
    map.set(w.repo, list);
  }
  return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
}

type FilterBarProps = {
  counts: ReadonlyMap<WatchFilter, number>;
  filter: WatchFilter;
  query: string;
  onFilterChange: (filter: WatchFilter) => void;
  onQueryChange: (query: string) => void;
};

function FilterBar({ counts, filter, query, onFilterChange, onQueryChange }: FilterBarProps) {
  return (
    <div className="flex flex-wrap items-center gap-1.5 px-6.5 pt-3">
      <ToggleGroup
        type="single"
        spacing={1.5}
        aria-label="Filter by state"
        value={filter}
        onValueChange={(value) => {
          if (isWatchFilter(value)) onFilterChange(value);
        }}
      >
        {watchFilterValues.map((value) => (
          <ToggleGroupItem
            key={value}
            value={value}
            className="h-7 gap-1.5 rounded-full border border-border px-3 text-body font-medium text-muted-foreground hover:bg-accent hover:text-foreground data-[state=on]:border-transparent data-[state=on]:bg-foreground data-[state=on]:text-background data-[state=on]:hover:bg-foreground/90"
          >
            {watchFilters[value].label} <span className="font-mono text-2xs opacity-80">{counts.get(value)}</span>
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <SearchInput
        aria-label="Filter by title, number, author, repository or label"
        placeholder="Filter by title, #number, author or label"
        value={query}
        onChange={(event) => onQueryChange(event.target.value)}
        className="ml-auto w-70"
      />
    </div>
  );
}

type Props = {
  enabled: boolean;
  onNavigate: Navigate;
  onWatchPR: () => void;
  onAddRepo: () => void;
};

export function WatchingView({ enabled, onNavigate, onWatchPR, onAddRepo }: Props) {
  const watches = useWatches(enabled);
  const pulls = usePullsByLabel(enabled);

  const [filter, setFilter] = useState<WatchFilter>("all");
  const [query, setQuery] = useState("");

  const sorted = useMemo(() => [...(watches.data ?? [])].sort(needsYouFirst), [watches.data]);
  const { counts, pinned, groups } = useMemo(() => {
    const needle = searchNeedle(query);
    const found = sorted.filter((w) => watchSearchText(w, pulls.byLabel.get(watchLabel(w))).includes(needle));
    const needsYou: Watch[] = [];
    const rest: Watch[] = [];
    for (const w of found.filter(watchFilters[filter].matches)) (needsAttention(w) ? needsYou : rest).push(w);
    return { counts: countByFilter(found), pinned: needsYou, groups: byRepo(rest) };
  }, [sorted, filter, query, pulls.byLabel]);
  const nothingMatches = pinned.length + groups.length === 0;

  function clearFilters() {
    setFilter("all");
    setQuery("");
  }

  function renderRow(w: Watch) {
    return (
      <WatchRow
        key={w.id}
        watch={w}
        pull={pulls.byLabel.get(watchLabel(w))}
        onOpen={() => onNavigate({ kind: "watch", id: w.id })}
      />
    );
  }

  const title = <h1 className="text-lg font-medium tracking-tight">Watched pull requests</h1>;

  if (watches.error) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <div className="p-5">
          <Alert variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>{watches.error.message}</AlertTitle>
          </Alert>
        </div>
      </>
    );
  }

  if (watches.isPending) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <div className="flex flex-col gap-3 p-5" role="status" aria-label="Loading watched pull requests">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      </>
    );
  }

  const all = watches.data ?? [];
  if (all.length === 0) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <FirstRun onWatchPR={onWatchPR} onAddRepo={onAddRepo} />
      </>
    );
  }

  return (
    <div className="flex flex-col">
      <ViewHeader>
        {title}
        <Meta>{all.length} active</Meta>
      </ViewHeader>

      <PullsErrorAlert error={pulls.error} />

      <FilterBar counts={counts} filter={filter} query={query} onFilterChange={setFilter} onQueryChange={setQuery} />
      {nothingMatches ? (
        <Empty className="py-16">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <SearchIcon />
            </EmptyMedia>
            <EmptyTitle>No watch matches</EmptyTitle>
            <EmptyDescription>No watched pull request has this state or text.</EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button variant="outline" onClick={clearFilters}>
              Clear the filters
            </Button>
          </EmptyContent>
        </Empty>
      ) : (
        <div className="flex flex-col gap-3 p-3">
          {pinned.length > 0 ? (
            <InboxGroup heading={`Needs you · ${pinned.length}`}>{pinned.map(renderRow)}</InboxGroup>
          ) : null}
          {groups.map(([name, list]) => (
            <InboxGroup
              key={name}
              heading={
                <button
                  type="button"
                  onClick={() => onNavigate({ kind: "repo", name })}
                  className="transition-colors hover:text-foreground"
                >
                  {name} · {list.length}
                </button>
              }
            >
              {list.map(renderRow)}
            </InboxGroup>
          ))}
        </div>
      )}
    </div>
  );
}
