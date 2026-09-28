import { appendFileSync, mkdirSync, renameSync, rmSync, statSync } from "node:fs";
import path from "node:path";
import type { LogAttr, LogLevel, LogRecord } from "../shared/logs";

const DEFAULT_KEEP = 2000;
const DEFAULT_MAX_BYTES = 5 * 1024 * 1024;
const BACKUP_SUFFIX = ".1";

export type AppLogOptions = {
  file: string;
  keep?: number;
  maxBytes?: number;
  echo?: (line: string) => void;
  now?: () => Date;
};

export type LogAttrs = Record<string, string | number | boolean>;

export class AppLog {
  private readonly keep: number;
  private readonly maxBytes: number;
  private readonly echo: (line: string) => void;
  private readonly now: () => Date;
  private readonly listeners = new Set<(record: LogRecord) => void>();
  private kept: LogRecord[] = [];
  private seq = 0;
  private size: number | null = null;

  constructor(private readonly opts: AppLogOptions) {
    this.keep = opts.keep ?? DEFAULT_KEEP;
    this.maxBytes = opts.maxBytes ?? DEFAULT_MAX_BYTES;
    this.echo = opts.echo ?? (() => undefined);
    this.now = opts.now ?? (() => new Date());
  }

  get file(): string {
    return this.opts.file;
  }

  get folder(): string {
    return path.dirname(this.opts.file);
  }

  debug(msg: string, attrs?: LogAttrs) {
    this.add("debug", msg, attrs);
  }

  info(msg: string, attrs?: LogAttrs) {
    this.add("info", msg, attrs);
  }

  warn(msg: string, attrs?: LogAttrs) {
    this.add("warn", msg, attrs);
  }

  error(msg: string, attrs?: LogAttrs) {
    this.add("error", msg, attrs);
  }

  records(): LogRecord[] {
    return [...this.kept];
  }

  onRecord(listener: (record: LogRecord) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private add(level: LogLevel, msg: string, attrs: LogAttrs = {}) {
    const record: LogRecord = {
      seq: ++this.seq,
      time: this.now().toISOString(),
      level,
      msg,
      attrs: toAttrs(attrs),
    };
    this.kept.push(record);
    if (this.kept.length > this.keep) this.kept = this.kept.slice(-this.keep);
    this.echo(`${level.toUpperCase()} ${msg}`);
    this.write(record);
    for (const listener of this.listeners) listener(record);
  }

  private write(record: LogRecord) {
    const line = `${JSON.stringify(record)}\n`;
    const bytes = Buffer.byteLength(line);
    try {
      this.makeRoomFor(bytes);
      appendFileSync(this.opts.file, line, { mode: 0o600 });
      this.size = (this.size ?? 0) + bytes;
    } catch {
      this.size = null;
    }
  }

  private makeRoomFor(bytes: number) {
    if (this.size === null) {
      mkdirSync(this.folder, { recursive: true, mode: 0o750 });
      this.size = fileSize(this.opts.file);
    }
    const fits = this.size === 0 || this.size + bytes <= this.maxBytes;
    if (fits) return;
    this.rotate();
    this.size = 0;
  }

  private rotate() {
    const backup = this.opts.file + BACKUP_SUFFIX;
    try {
      rmSync(backup, { force: true });
      renameSync(this.opts.file, backup);
    } catch {}
  }
}

function toAttrs(attrs: LogAttrs): LogAttr[] {
  return Object.entries(attrs).map(([key, value]) => ({ key, value: String(value) }));
}

function fileSize(file: string): number {
  try {
    return statSync(file).size;
  } catch {
    return 0;
  }
}
