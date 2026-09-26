export type UpdateState =
  | "unsupported"
  | "idle"
  | "checking"
  | "available"
  | "not-available"
  | "downloading"
  | "downloaded"
  | "installing"
  | "error";

export type UpdateStatus = {
  state: UpdateState;
  currentVersion: string;
  version?: string;
  percent?: number;
  message?: string;
  checkedAt?: string;
};

export type UpdateChannel = "stable" | "prerelease";

export type UpdateSettings = {
  autoDownload: boolean;
  channel: UpdateChannel;
};

export const RELEASES_URL = "https://github.com/deividfortuna/babysitter/releases";

const BUSY_STATES: UpdateState[] = ["checking", "downloading", "downloaded", "installing"];

export function isBusy(state: UpdateState): boolean {
  return BUSY_STATES.includes(state);
}

export function isUpdateChannel(value: unknown): value is UpdateChannel {
  return value === "stable" || value === "prerelease";
}

export function defaultChannel(currentVersion: string): UpdateChannel {
  return currentVersion.includes("-") ? "prerelease" : "stable";
}

export function resolveSettings(saved: Partial<UpdateSettings>, currentVersion: string): UpdateSettings {
  return {
    autoDownload: saved.autoDownload ?? true,
    channel: saved.channel ?? defaultChannel(currentVersion),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function parseSettingsPatch(raw: unknown): Partial<UpdateSettings> {
  if (!isRecord(raw)) return {};
  return {
    ...(typeof raw.autoDownload === "boolean" ? { autoDownload: raw.autoDownload } : {}),
    ...(isUpdateChannel(raw.channel) ? { channel: raw.channel } : {}),
  };
}

export function releaseUrl(version: string): string {
  return `${RELEASES_URL}/tag/v${version}`;
}
