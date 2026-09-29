import { useLayoutEffect, useRef, useState } from "react";
import { CheckIcon, CopyIcon, FolderOpenIcon } from "lucide-react";
import { OptionSelect, type Option } from "@/components/option-select";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useCopy } from "@/hooks/useCopy";
import { useAppLogs, useDaemonLogs } from "@/hooks/useLogs";
import { bridge } from "@/lib/bridge";
import { cn } from "@/lib/utils";
import { atLeast, type LogAttr, type LogLevel, type LogRecord } from "../../shared/logs";

type Source = "daemon" | "app";

const LEVEL_OPTIONS: Option<LogLevel>[] = [
  { value: "debug", label: "All levels" },
  { value: "info", label: "Info and above" },
  { value: "warn", label: "Warnings and errors" },
  { value: "error", label: "Errors only" },
];

const LEVEL_STYLES: Record<LogLevel, string> = {
  debug: "text-muted-foreground",
  info: "text-foreground",
  warn: "font-semibold text-foreground",
  error: "font-semibold text-destructive",
};

const STICK_TO_END_PX = 24;

function formatAttrs(attrs: LogAttr[]): string {
  return attrs.map((a) => ` ${a.key}=${a.value}`).join("");
}

export function formatLogLine(record: LogRecord): string {
  return `${record.time} ${record.level.toUpperCase()} ${record.msg}${formatAttrs(record.attrs)}`;
}

function clockTime(iso: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return iso;
  const pad = (n: number, width = 2) => String(n).padStart(width, "0");
  return `${pad(at.getHours())}:${pad(at.getMinutes())}:${pad(at.getSeconds())}.${pad(at.getMilliseconds(), 3)}`;
}

function matches(record: LogRecord, min: LogLevel, needle: string): boolean {
  if (!atLeast(record.level, min)) return false;
  if (!needle) return true;
  return formatLogLine(record).toLowerCase().includes(needle);
}

export function LogsViewer({ className }: { className?: string }) {
  const [source, setSource] = useState<Source>("daemon");
  const [min, setMin] = useState<LogLevel>("debug");
  const [search, setSearch] = useState("");
  const [folderError, setFolderError] = useState<string | null>(null);
  const daemon = useDaemonLogs(source === "daemon");
  const app = useAppLogs(source === "app");
  const { copied, copy } = useCopy();

  const records = source === "daemon" ? daemon.records : app;
  const needle = search.trim().toLowerCase();
  const shown = records.filter((record) => matches(record, min, needle));
  const waitingForDaemon = source === "daemon" && !daemon.connected;

  async function openFolder() {
    const result = await bridge.logs.openFolder();
    setFolderError(result.ok ? null : result.error);
  }

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="flex flex-wrap items-center gap-2">
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          aria-label="Log source"
          value={source}
          onValueChange={(value) => {
            if (value === "daemon" || value === "app") setSource(value);
          }}
        >
          <ToggleGroupItem value="daemon">Daemon</ToggleGroupItem>
          <ToggleGroupItem value="app">App</ToggleGroupItem>
        </ToggleGroup>
        <OptionSelect label="Lowest level shown" options={LEVEL_OPTIONS} value={min} onChange={setMin} />
        <Input
          type="search"
          aria-label="Filter the log"
          placeholder="Filter"
          className="h-8 w-40 flex-1"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <Button
          variant="outline"
          size="sm"
          disabled={shown.length === 0}
          onClick={() => void copy(shown.map(formatLogLine).join("\n"))}
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? "Copied" : "Copy"}
        </Button>
        {bridge.logs.desktop ? (
          <Button variant="outline" size="sm" onClick={() => void openFolder()}>
            <FolderOpenIcon />
            Open folder
          </Button>
        ) : null}
      </div>
      {folderError ? <p className="text-sm text-destructive">{folderError}</p> : null}
      <LogLines records={shown} empty={waitingForDaemon ? "Waiting for the daemon." : "No records to show."} />
    </div>
  );
}

function LogLines({ records, empty }: { records: LogRecord[]; empty: string }) {
  const box = useRef<HTMLDivElement>(null);
  const atEnd = useRef(true);

  useLayoutEffect(() => {
    const el = box.current;
    if (el && atEnd.current) el.scrollTop = el.scrollHeight;
  }, [records]);

  function trackEnd() {
    const el = box.current;
    if (!el) return;
    atEnd.current = el.scrollHeight - el.scrollTop - el.clientHeight < STICK_TO_END_PX;
  }

  return (
    <div
      ref={box}
      role="log"
      aria-label="Log records"
      onScroll={trackEnd}
      className="min-h-40 flex-1 overflow-auto rounded-md border bg-muted/30 p-2 font-mono text-2xs/relaxed"
    >
      {records.length === 0 ? (
        <p className="p-2 text-muted-foreground">{empty}</p>
      ) : (
        records.map((record) => (
          <div key={record.seq} className="break-all whitespace-pre-wrap">
            <span className="text-muted-foreground">{clockTime(record.time)}</span>{" "}
            <span className={cn("inline-block w-11 uppercase", LEVEL_STYLES[record.level])}>{record.level}</span>{" "}
            <span className={record.level === "error" ? "text-destructive" : undefined}>{record.msg}</span>
            {record.attrs.length > 0 ? (
              <span className="text-muted-foreground">{formatAttrs(record.attrs)}</span>
            ) : null}
          </div>
        ))
      )}
    </div>
  );
}
