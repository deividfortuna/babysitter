import { useState } from "react";
import { useAlwaysShowRateLimit } from "@/hooks/use-always-show-rate-limit";
import { useNow } from "@/hooks/use-now";
import { useRateLimit, type RateLimit } from "@/hooks/useRateLimit";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { cn } from "@/lib/utils";

type KnownState = Exclude<RateLimit["state"], "unknown">;

type Copy = {
  title: string;
  note?: string;
};

const COPY: Record<KnownState, Copy> = {
  ok: { title: "GitHub API" },
  low: {
    title: "Rate limit nearly used",
    note: "Polling pauses before the limit runs out, then goes on by itself at the reset.",
  },
  paused: {
    title: "Polling paused",
    note: "The GitHub rate limit is nearly used. Every watch picks up where it left off at the reset, and nothing is lost.",
  },
  slowed: {
    title: "GitHub asked to slow down",
    note: "Too many requests in a short time. The daemon waits the minute GitHub asked for and goes on.",
  },
};

const TICK_MS = 15_000;

const RELEVANT_USED_PERCENT = 50;

const count = new Intl.NumberFormat("en-US");

function minutesUntil(iso: string, now: number): number {
  return Math.max(1, Math.ceil((new Date(iso).getTime() - now) / 60_000));
}

function countdown(rate: RateLimit, now: number): string | null {
  if (rate.state === "slowed" && rate.retryAt) return `retry in ${minutesUntil(rate.retryAt, now)}m`;
  if (rate.resetAt) return `resets in ${minutesUntil(rate.resetAt, now)}m`;
  return null;
}

function usedPercent(rate: RateLimit): number {
  if (rate.limit <= 0) return 0;
  return Math.round(((rate.limit - rate.remaining) / rate.limit) * 1000) / 10;
}

function isRelevant(rate: RateLimit): boolean {
  const used = rate.limit - rate.remaining;
  return rate.state !== "ok" || used * 100 > rate.limit * RELEVANT_USED_PERCENT;
}

function episodeOf(rate: RateLimit): string {
  return [rate.state, rate.resetAt ?? "", rate.retryAt ?? ""].join("|");
}

type RateLimitCardProps = {
  enabled: boolean;
  onPollLessOften: () => void;
};

export function RateLimitCard({ enabled, onPollLessOften }: RateLimitCardProps) {
  const rateLimit = useRateLimit(enabled);
  const { alwaysShow } = useAlwaysShowRateLimit();
  const now = useNow(TICK_MS);
  const [dismissed, setDismissed] = useState<string | null>(null);

  const rate = rateLimit.data;
  if (!rate || rate.state === "unknown") return null;
  if (!alwaysShow && !isRelevant(rate)) return null;

  const { title, note } = COPY[rate.state];
  const episode = episodeOf(rate);
  const showNote = note !== undefined && dismissed !== episode;
  const when = countdown(rate, now);
  const used = usedPercent(rate);

  return (
    <Card role="region" aria-label="GitHub rate limit" className="gap-2.5 py-3">
      <CardHeader className="flex items-baseline justify-between gap-2 px-3">
        <CardTitle className="min-w-0 text-sm/5">{title}</CardTitle>
        {when ? (
          <CardAction className="shrink-0 font-mono text-2xs/4 whitespace-nowrap text-muted-foreground">
            {when}
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent className="flex flex-col gap-2 px-3">
        <Progress
          aria-label="GitHub requests used this hour"
          aria-valuenow={used}
          value={used}
          className={cn(
            "bg-muted",
            rate.state === "paused"
              ? "**:data-[slot=progress-indicator]:bg-destructive"
              : "**:data-[slot=progress-indicator]:bg-muted-foreground",
          )}
        />
        <span className="font-mono text-2xs/4 text-muted-foreground">
          {count.format(rate.remaining)} of {count.format(rate.limit)} left
        </span>
        {showNote ? <p className="text-body/4.5 text-muted-foreground">{note}</p> : null}
      </CardContent>
      {showNote ? (
        <CardFooter className="justify-between gap-1.5 px-3">
          <Button variant="outline" size="sm" className="h-7 text-body" onClick={onPollLessOften}>
            Poll less often
          </Button>
          <Button variant="secondary" size="sm" className="h-7 text-body" onClick={() => setDismissed(episode)}>
            Dismiss
          </Button>
        </CardFooter>
      ) : null}
    </Card>
  );
}
