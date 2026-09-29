export type Intervals = {
  pollIntervalSeconds: number;
  watchIntervalSeconds: number;
  watchMaxIntervalSeconds: number;
  checkMaxIntervalSeconds: number;
};

export type PresetId = "relaxed" | "balanced" | "eager";

export type Preset = {
  id: PresetId;
  label: string;
  intervals: Intervals;
};

const MINUTE = 60;

export const PRESETS: readonly Preset[] = [
  {
    id: "relaxed",
    label: "Relaxed",
    intervals: {
      pollIntervalSeconds: 5 * MINUTE,
      watchIntervalSeconds: 10 * MINUTE,
      watchMaxIntervalSeconds: 30 * MINUTE,
      checkMaxIntervalSeconds: 30 * MINUTE,
    },
  },
  {
    id: "balanced",
    label: "Balanced",
    intervals: {
      pollIntervalSeconds: MINUTE,
      watchIntervalSeconds: 3 * MINUTE,
      watchMaxIntervalSeconds: 15 * MINUTE,
      checkMaxIntervalSeconds: 15 * MINUTE,
    },
  },
  {
    id: "eager",
    label: "Eager",
    intervals: {
      pollIntervalSeconds: 30,
      watchIntervalSeconds: MINUTE,
      watchMaxIntervalSeconds: 5 * MINUTE,
      checkMaxIntervalSeconds: 5 * MINUTE,
    },
  },
];

export const DEFAULT_HOURLY_LIMIT = 5000;

const INTERVAL_KEYS: readonly (keyof Intervals)[] = [
  "pollIntervalSeconds",
  "watchIntervalSeconds",
  "watchMaxIntervalSeconds",
  "checkMaxIntervalSeconds",
];

export function presetOf(intervals: Intervals): Preset | undefined {
  return PRESETS.find((preset) => INTERVAL_KEYS.every((key) => preset.intervals[key] === intervals[key]));
}

export function fasterPreset(id: PresetId | undefined): Preset | undefined {
  const at = PRESETS.findIndex((preset) => preset.id === id);
  return at < 0 ? undefined : PRESETS[at + 1];
}

export function shortInterval(seconds: number): string {
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % MINUTE === 0) return `${seconds / MINUTE}m`;
  if (seconds > MINUTE) return `${Math.floor(seconds / MINUTE)}m ${seconds % MINUTE}s`;
  return `${seconds}s`;
}

export function presetSummary(intervals: Intervals): string {
  return [intervals.pollIntervalSeconds, intervals.watchIntervalSeconds, intervals.watchMaxIntervalSeconds]
    .map(shortInterval)
    .join(" · ");
}

export type Load = {
  repos: number;
  watches: number;
};

export function requestsPerHour(intervals: Intervals, load: Load): number {
  const repoPasses = (load.repos * 3600) / intervals.pollIntervalSeconds;
  const watchPolls = (load.watches * 3600) / intervals.watchIntervalSeconds;
  return repoPasses + watchPolls;
}

export function roughCount(requests: number): number {
  const step = requests < 100 ? 5 : requests < 1000 ? 10 : 50;
  return Math.round(requests / step) * step;
}

export function shareOf(requests: number, limit: number): number {
  return Math.min(1, requests / limit);
}
