import { Fragment, useMemo, useState, type CSSProperties } from "react";
import { ChevronRightIcon } from "lucide-react";
import {
  DaemonSettings,
  DraftNumberRow,
  SettingsCard,
  SettingsSection,
  useTrackedWrite,
} from "@/components/settings-page";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { count } from "@/components/proposal-dialogs";
import { Separator } from "@/components/ui/separator";
import { useDraftField } from "@/hooks/use-draft-field";
import { useRateLimit } from "@/hooks/useRateLimit";
import { useRepos } from "@/hooks/useRepos";
import type { Settings } from "@/hooks/useSettings";
import { useWatches } from "@/hooks/useWatches";
import {
  DEFAULT_HOURLY_LIMIT,
  PRESETS,
  fasterPreset,
  presetOf,
  presetSummary,
  requestsPerHour,
  roughCount,
  shareOf,
  shortInterval,
  watchIntervalPatch,
  type Intervals,
  type Preset,
} from "@/lib/polling";
import { wholeNumber } from "@/lib/approvals";
import { cn } from "@/lib/utils";

const MIN_SECONDS = 10;
const MAX_SECONDS = 86_400;

function secondsFrom(min: number) {
  return (text: string): number | undefined => {
    const seconds = wholeNumber(text);
    if (seconds === undefined) return undefined;
    const inRange = seconds >= min && seconds <= MAX_SECONDS;
    return inRange ? seconds : undefined;
  };
}

const WATCH_INTERVAL_TEXT = "Seconds between polls of a pull request under watch.";

const PACE_ROWS: { label: string; seconds: (intervals: Intervals) => number; suffix: string }[] = [
  { label: "Pass over the repositories you watch", seconds: (i) => i.pollIntervalSeconds, suffix: "" },
  { label: "Poll a pull request under watch", seconds: (i) => i.watchIntervalSeconds, suffix: "" },
  { label: "Slow a quiet pull request down to", seconds: (i) => i.watchMaxIntervalSeconds, suffix: " at most" },
];

export function PollingPanel() {
  return <DaemonSettings>{(settings) => <PollingForm settings={settings} />}</DaemonSettings>;
}

function PollingForm({ settings }: { settings: Settings }) {
  const save = useTrackedWrite();
  const intervals: Intervals = settings;
  const preset = presetOf(intervals);
  const [byHand, setByHand] = useState(!preset);
  const [raisedTo, setRaisedTo] = useState<number | null>(null);

  const longestRaised = raisedTo !== null && raisedTo === intervals.watchMaxIntervalSeconds;

  function saveWatchInterval(watchIntervalSeconds: number) {
    const patch = watchIntervalPatch(watchIntervalSeconds, intervals);
    setRaisedTo(patch.watchMaxIntervalSeconds ?? null);
    save(patch);
  }

  function savePreset(choice: Preset) {
    setRaisedTo(null);
    save(choice.intervals);
  }

  return (
    <div className="flex flex-col gap-4.5">
      <div role="group" aria-label="Polling speed" className="flex gap-2">
        {PRESETS.map((choice) => (
          <SpeedTile
            key={choice.id}
            label={choice.label}
            detail={presetSummary(choice.intervals)}
            pressed={preset?.id === choice.id}
            onClick={() => savePreset(choice)}
          />
        ))}
        <SpeedTile label="Custom" detail="your values" pressed={!preset} onClick={() => setByHand(true)} />
      </div>

      <Budget intervals={intervals} preset={preset} />

      <SettingsSection label={preset ? "What a preset sets" : "What your values set"}>
        <dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-2 px-0.5 text-sm">
          {PACE_ROWS.map((row) => (
            <Fragment key={row.label}>
              <dt>{row.label}</dt>
              <dd className="font-mono text-2xs text-muted-foreground">
                every {shortInterval(row.seconds(intervals))}
                {row.suffix}
              </dd>
            </Fragment>
          ))}
        </dl>
      </SettingsSection>

      <Separator />

      <Collapsible open={byHand} onOpenChange={setByHand} className="flex flex-col gap-2">
        <CollapsibleTrigger className="group/hand flex h-9 items-center gap-2 px-0.5 text-sm font-medium">
          <ChevronRightIcon className="size-4 transition-transform group-data-[state=open]/hand:rotate-90" />
          Set the intervals by hand
        </CollapsibleTrigger>
        <CollapsibleContent>
          <SettingsCard>
            <SecondsRow
              id="poll-interval"
              label="Repository poll interval"
              description="Seconds between passes over the repositories you watch."
              value={intervals.pollIntervalSeconds}
              onCommit={(pollIntervalSeconds) => save({ pollIntervalSeconds })}
            />
            <SecondsRow
              id="check-max-interval"
              label="Longest check read interval"
              description="Seconds the repository poll waits at most between two reads of checks that still run. The wait doubles after each read, and a new commit or a manual sync starts it again."
              value={intervals.checkMaxIntervalSeconds}
              onCommit={(checkMaxIntervalSeconds) => save({ checkMaxIntervalSeconds })}
            />
            <SecondsRow
              id="watch-interval"
              label="Watch poll interval"
              description={
                longestRaised
                  ? `${WATCH_INTERVAL_TEXT} The longest watch poll interval moved to ${shortInterval(raisedTo)} too.`
                  : WATCH_INTERVAL_TEXT
              }
              value={intervals.watchIntervalSeconds}
              onCommit={saveWatchInterval}
            />
            <SecondsRow
              id="watch-max-interval"
              label="Longest watch poll interval"
              description="Seconds a quiet pull request waits between polls at most. The wait doubles after each poll where nothing happens. The same value as the watch poll interval keeps one fixed interval."
              value={intervals.watchMaxIntervalSeconds}
              min={intervals.watchIntervalSeconds}
              onCommit={(watchMaxIntervalSeconds) => save({ watchMaxIntervalSeconds })}
            />
          </SettingsCard>
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}

type SpeedTileProps = {
  label: string;
  detail: string;
  pressed: boolean;
  onClick: () => void;
};

function SpeedTile({ label, detail, pressed, onClick }: SpeedTileProps) {
  return (
    <button
      type="button"
      aria-pressed={pressed}
      onClick={onClick}
      className={cn(
        "flex min-w-0 flex-1 flex-col items-start gap-0.5 rounded-lg border bg-background px-3 py-2.5 text-left transition-colors outline-none hover:bg-muted/40 focus-visible:ring-3 focus-visible:ring-ring/50",
        pressed && "border-primary ring-1 ring-primary",
      )}
    >
      <span className="text-sm font-medium">{label}</span>
      <span className="font-mono text-2xs text-muted-foreground">{detail}</span>
    </button>
  );
}

function Budget({ intervals, preset }: { intervals: Intervals; preset: Preset | undefined }) {
  const repos = useRepos(true).data?.length ?? 0;
  const watches = useWatches(true, "all").data?.filter((watch) => watch.status === "active").length ?? 0;
  const reported = useRateLimit(true).data?.limit ?? 0;
  const limit = reported > 0 ? reported : DEFAULT_HOURLY_LIMIT;
  const load = { repos, watches };

  const requests = requestsPerHour(intervals, load);
  const share = shareOf(requests, limit);
  const over = requests > limit;
  const faster = fasterPreset(preset?.id);
  const extra = faster ? shareOf(requestsPerHour(faster.intervals, load), limit) - share : 0;
  const fill = over ? "bg-destructive" : "bg-primary";

  return (
    <div className="flex flex-col gap-2 rounded-lg border bg-muted/25 p-3.5">
      <div className="flex flex-wrap items-baseline gap-x-2">
        <span className="text-sm font-medium">
          {over
            ? `More than the ${limit.toLocaleString("en")} GitHub requests an hour`
            : `About ${roughCount(requests).toLocaleString("en")} GitHub requests an hour`}
        </span>
        <span className="ml-auto font-mono text-2xs text-muted-foreground">
          of {limit.toLocaleString("en")} · with {count(repos, "repository", "repositories")} and{" "}
          {count(watches, "watch", "watches")}
        </span>
      </div>
      <div
        role="meter"
        aria-label="Share of the hourly rate limit"
        aria-valuemin={0}
        aria-valuemax={limit}
        aria-valuenow={Math.round(Math.min(requests, limit))}
        className="flex h-2 overflow-hidden rounded-full bg-muted"
      >
        <div className={cn("w-(--part)", fill)} style={{ "--part": `${share * 100}%` } as CSSProperties} />
        {extra > 0 ? (
          <div className="w-(--part) bg-primary/35" style={{ "--part": `${extra * 100}%` } as CSSProperties} />
        ) : null}
      </div>
      <div className="flex gap-4 font-mono text-2xs text-muted-foreground">
        <span className="inline-flex items-center gap-1.5">
          <span aria-hidden="true" className={cn("size-2 rounded-xs", fill)} />
          {preset?.label ?? "Your values"}
        </span>
        {extra > 0 ? (
          <span className="inline-flex items-center gap-1.5">
            <span aria-hidden="true" className="size-2 rounded-xs bg-primary/35" />
            {faster?.label} would add {Math.max(1, Math.round(extra * 100))}%
          </span>
        ) : null}
      </div>
    </div>
  );
}

type SecondsRowProps = {
  id: string;
  label: string;
  description: string;
  value: number;
  min?: number;
  onCommit: (seconds: number) => void;
};

function SecondsRow({ id, label, description, value, min = MIN_SECONDS, onCommit }: SecondsRowProps) {
  const parse = useMemo(() => secondsFrom(min), [min]);
  const field = useDraftField<number>({ value, format: String, parse, commit: onCommit });
  return (
    <DraftNumberRow
      id={id}
      label={label}
      description={description}
      error={`A whole number of seconds from ${min} to ${MAX_SECONDS}.`}
      min={min}
      max={MAX_SECONDS}
      field={field}
    />
  );
}
