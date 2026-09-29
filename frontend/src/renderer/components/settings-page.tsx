import { createContext, useCallback, useContext, useRef, useState, type ComponentProps, type ReactNode } from "react";
import { CheckIcon, CircleAlertIcon } from "lucide-react";
import { SettingRow } from "@/components/setting-row";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import type { DraftField } from "@/hooks/use-draft-field";
import { useSettings, useWriteSettings, type Settings } from "@/hooks/useSettings";
import { cn } from "@/lib/utils";

export type SaveState = {
  page: string;
  label: string;
  status: "saving" | "saved" | "failed";
  message?: string;
} | null;

type Track = (work: Promise<unknown>) => void;

const TrackContext = createContext<Track>((work) => {
  work.catch(() => undefined);
});

export function useTrackSave(): Track {
  return useContext(TrackContext);
}

function keepSaving(state: SaveState): SaveState {
  return state?.status === "saving" ? state : null;
}

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : "Could not save the change.";
}

export function useSaveState() {
  const [state, setState] = useState<SaveState>(null);
  const latest = useRef(0);
  const tracks = useRef(new Map<string, Track>());

  const trackFor = useCallback((page: string, label: string): Track => {
    const known = tracks.current.get(page);
    if (known) return known;
    const track: Track = (work) => {
      const turn = ++latest.current;
      setState({ page, label, status: "saving" });
      work.then(
        () => {
          if (turn === latest.current) setState({ page, label, status: "saved" });
        },
        (error: unknown) => {
          if (turn === latest.current) setState({ page, label, status: "failed", message: messageOf(error) });
        },
      );
    };
    tracks.current.set(page, track);
    return track;
  }, []);

  const settle = useCallback(() => setState(keepSaving), []);
  const clear = useCallback(() => setState(null), []);

  return { state, trackFor, settle, clear };
}

export function SaveTracker({ track, children }: { track: Track; children: ReactNode }) {
  return <TrackContext.Provider value={track}>{children}</TrackContext.Provider>;
}

const MARKS = {
  saving: { icon: <Spinner className="size-3.5" />, word: "saving" },
  saved: { icon: <CheckIcon className="size-3.5 text-success" />, word: "saved" },
};

export function SaveMark({ state, page }: { state: SaveState; page: string }) {
  const onThisPage = state?.page === page;
  if (!onThisPage || state.status === "failed") return null;
  const { icon, word } = MARKS[state.status];
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

function elsewhereText({ label, status, message }: NonNullable<SaveState>): string {
  if (status === "failed") return `${label}: ${message}`;
  return `${label} ${status}`;
}

export function SaveElsewhere({ state, page }: { state: SaveState; page: string }) {
  const elsewhere = state !== null && state.page !== page;
  if (!elsewhere) return null;
  return (
    <p
      role="status"
      className={cn("px-2.5 text-2xs/snug text-muted-foreground", state.status === "failed" && "text-destructive")}
    >
      {elsewhereText(state)}
    </p>
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
  const { write, error } = useWriteSettings();
  const track = useTrackSave();
  const save = useCallback(
    (patch: Partial<Settings>) => {
      track(write(patch));
    },
    [write, track],
  );
  return { save, error: error?.message };
}

export function DaemonSettings({ children }: { children: (settings: Settings) => ReactNode }) {
  const settings = useSettings();
  if (settings.isError) return <SettingsError message={settings.error.message} />;
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
