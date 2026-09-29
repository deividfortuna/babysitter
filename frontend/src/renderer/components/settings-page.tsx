import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import { CheckIcon, CircleAlertIcon } from "lucide-react";
import { SettingRow } from "@/components/setting-row";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import type { DraftField } from "@/hooks/use-draft-field";
import { useSettings, useWriteSettings, type Settings } from "@/hooks/useSettings";
import { apiErrorMessage } from "@/lib/api-client";
import { cn } from "@/lib/utils";

type Mark = { page: string; label: string; status: "saving" | "saved" } | null;

type Failure = { label: string; message: string; at: number };

export type SaveState = { mark: Mark; failures: Record<string, Failure> };

const NOTHING_SAVED: SaveState = { mark: null, failures: {} };

type Track = (work: Promise<unknown>) => void;

type PageTrack = (page: string, label: string, work: Promise<unknown>) => void;

const TrackContext = createContext<Track>((work) => {
  work.catch(() => undefined);
});

export function useTrackSave(): Track {
  return useContext(TrackContext);
}

function withoutFailureBefore(failures: Record<string, Failure>, page: string, started: number) {
  const failure = failures[page];
  if (!failure || failure.at > started) return failures;
  const { [page]: _cleared, ...rest } = failures;
  return rest;
}

export function useSaveState() {
  const [state, setState] = useState<SaveState>(NOTHING_SAVED);
  const clock = useRef(0);
  const newest = useRef(0);

  const track = useCallback<PageTrack>((page, label, work) => {
    const started = ++clock.current;
    newest.current = started;
    setState((current) => ({ ...current, mark: { page, label, status: "saving" } }));
    work.then(
      () => {
        const isNewest = newest.current === started;
        setState((current) => ({
          mark: isNewest ? { page, label, status: "saved" } : current.mark,
          failures: withoutFailureBefore(current.failures, page, started),
        }));
      },
      (error: unknown) => {
        const isNewest = newest.current === started;
        const failure = { label, message: apiErrorMessage(error, "Could not save the change."), at: ++clock.current };
        setState((current) => ({
          mark: isNewest ? null : current.mark,
          failures: { ...current.failures, [page]: failure },
        }));
      },
    );
  }, []);

  const settle = useCallback(
    () => setState((current) => ({ ...current, mark: current.mark?.status === "saving" ? current.mark : null })),
    [],
  );
  const clear = useCallback(() => setState(NOTHING_SAVED), []);

  return { state, track, settle, clear };
}

type SaveTrackerProps = {
  page: string;
  label: string;
  track: PageTrack;
  children: ReactNode;
};

export function SaveTracker({ page, label, track, children }: SaveTrackerProps) {
  const bound = useMemo<Track>(() => (work) => track(page, label, work), [track, page, label]);
  return <TrackContext.Provider value={bound}>{children}</TrackContext.Provider>;
}

const MARKS = {
  saving: { icon: <Spinner className="size-3.5" />, word: "saving" },
  saved: { icon: <CheckIcon className="size-3.5 text-success" />, word: "saved" },
};

export function SaveMark({ state, page }: { state: SaveState; page: string }) {
  const { mark } = state;
  if (mark?.page !== page) return null;
  const { icon, word } = MARKS[mark.status];
  return (
    <span
      role="status"
      className="inline-flex h-6 shrink-0 items-center gap-1 font-mono text-2xs text-muted-foreground"
    >
      {icon}
      {word}
    </span>
  );
}

export function SaveFailure({ state, page }: { state: SaveState; page: string }) {
  return <SettingsError message={state.failures[page]?.message} />;
}

export function SaveElsewhere({ state, page }: { state: SaveState; page: string }) {
  const { mark } = state;
  const markElsewhere = mark !== null && mark.page !== page;
  const failures = Object.entries(state.failures)
    .filter(([failedPage]) => failedPage !== page)
    .map(([, failure]) => failure)
    .sort((a, b) => b.at - a.at);
  return (
    <>
      {markElsewhere ? (
        <p role="status" className="px-2.5 text-2xs/snug text-muted-foreground">
          {mark.label} {mark.status}
        </p>
      ) : null}
      {failures.map((failure) => (
        <p key={failure.label} role="status" className="px-2.5 text-2xs/snug text-destructive">
          {failure.label}: {failure.message}
        </p>
      ))}
    </>
  );
}

export function SettingsSection({ label, children }: { label: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-1.5">
      <h3 className="px-0.5 text-body font-medium text-muted-foreground">{label}</h3>
      {children}
    </section>
  );
}

export function SettingsCard({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={cn("divide-y rounded-lg border bg-muted/25", className)}>{children}</div>;
}

export function SettingsRow({ className, ...props }: ComponentProps<typeof SettingRow>) {
  return <SettingRow {...props} className={cn("px-3.5 py-3", className)} />;
}

type DraftNumberRowProps = {
  id: string;
  label: string;
  description: string;
  error: string;
  min: number;
  max?: number;
  field: DraftField;
};

export function DraftNumberRow({ id, label, description, error, min, max, field }: DraftNumberRowProps) {
  return (
    <SettingsRow
      label={label}
      htmlFor={id}
      description={field.invalid ? <span className="text-destructive">{error}</span> : description}
    >
      <Input
        id={id}
        type="number"
        inputMode="numeric"
        min={min}
        max={max}
        className="w-24"
        aria-invalid={field.invalid || undefined}
        value={field.text}
        onChange={(e) => field.change(e.target.value)}
        onBlur={field.flush}
      />
    </SettingsRow>
  );
}

export function useTrackedWrite() {
  const write = useWriteSettings();
  const track = useTrackSave();
  return useCallback((patch: Partial<Settings>) => track(write(patch)), [write, track]);
}

export function DaemonSettings({ children }: { children: (settings: Settings) => ReactNode }) {
  const settings = useSettings();
  if (settings.error) return <SettingsError message={settings.error.message} />;
  if (!settings.data) return <Spinner />;
  return children(settings.data);
}

export function SettingsError({ message }: { message: string | null | undefined }) {
  if (!message) return null;
  return (
    <Alert variant="destructive">
      <CircleAlertIcon />
      <AlertTitle>{message}</AlertTitle>
    </Alert>
  );
}
