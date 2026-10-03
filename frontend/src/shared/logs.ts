export const LOG_LEVELS = ["debug", "info", "warn", "error"] as const;

export type LogLevel = (typeof LOG_LEVELS)[number];

export type LogAttr = { key: string; value: string };

export type LogRecord = {
  seq: number;
  time: string;
  level: LogLevel;
  msg: string;
  attrs: LogAttr[];
};

export function isDaemonLogRecord(line: string): boolean {
  return line.startsWith("time=");
}

export function atLeast(level: LogLevel, min: LogLevel): boolean {
  return LOG_LEVELS.indexOf(level) >= LOG_LEVELS.indexOf(min);
}
