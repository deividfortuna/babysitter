import { useMemo } from "react";
import { CircleAlertIcon, EyeIcon } from "lucide-react";
import { usePullsByLabel } from "@/hooks/usePulls";
import { useWatches, type Watch } from "@/hooks/useWatches";
import { FirstRun } from "@/components/first-run";
import { InboxGroup, PullsErrorAlert } from "@/components/inbox-row";
import { Meta } from "@/components/status-badges";
import { ViewHeader } from "@/components/view-header";
import { WatchRow } from "@/components/watch-row";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import type { Navigate } from "@/lib/navigation";
import { needsAttention, watchLabel } from "@/lib/watch-status";

const ALL = "__all__";

type Props = {
  enabled: boolean;
  repo?: string;
  onNavigate: Navigate;
  onWatchPR: () => void;
  onAddRepo: () => void;
};

export function WatchingView({ enabled, repo, onNavigate, onWatchPR, onAddRepo }: Props) {
  const watches = useWatches(enabled);
  const pulls = usePullsByLabel(enabled);

  const repos = useMemo(() => [...new Set((watches.data ?? []).map((w) => w.repo))].sort(), [watches.data]);
  const groups = useMemo(() => {
    const map = new Map<string, Watch[]>();
    for (const w of watches.data ?? []) {
      if (repo && w.repo !== repo) continue;
      const list = map.get(w.repo) ?? [];
      list.push(w);
      map.set(w.repo, list);
    }
    for (const list of map.values()) list.sort((a, b) => b.number - a.number);
    return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [watches.data, repo]);

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
  const needYou = all.filter(needsAttention).length;

  return (
    <div className="flex flex-col">
      <ViewHeader>
        {title}
        <Meta>
          {all.length} active · {needYou} need{needYou === 1 ? "s" : ""} you
        </Meta>
        <div className="ml-auto">
          <Select
            value={repo ?? ALL}
            onValueChange={(value) => onNavigate({ kind: "watching", repo: value === ALL ? undefined : value })}
          >
            <SelectTrigger size="sm" className="font-mono text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              <SelectGroup>
                <SelectItem value={ALL}>all repositories</SelectItem>
                {repos.map((r) => (
                  <SelectItem key={r} value={r}>
                    {r}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
      </ViewHeader>

      <PullsErrorAlert error={pulls.error} />

      {groups.length === 0 ? (
        <Empty className="py-16">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <EyeIcon />
            </EmptyMedia>
            <EmptyTitle>No watch in {repo}</EmptyTitle>
            <EmptyDescription>Pull requests of this repository show here once one is watched.</EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button variant="outline" onClick={() => onNavigate({ kind: "watching" })}>
              Show every repository
            </Button>
          </EmptyContent>
        </Empty>
      ) : (
        <div className="flex flex-col gap-3 p-3">
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
              {list.map((w) => (
                <WatchRow
                  key={w.id}
                  watch={w}
                  pull={pulls.byLabel.get(watchLabel(w))}
                  onOpen={() => onNavigate({ kind: "watch", id: w.id })}
                />
              ))}
            </InboxGroup>
          ))}
        </div>
      )}
    </div>
  );
}
