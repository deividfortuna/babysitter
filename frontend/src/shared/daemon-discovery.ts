export type RunFileInfo = {
  pid: number;
  port: number;
  startedAtMs: number;
  owner?: string;
  supervisor?: string;
  version?: string;
};

export function parseRunFile(contents: string): RunFileInfo | null {
  let raw: unknown;
  try {
    raw = JSON.parse(contents);
  } catch {
    return null;
  }
  if (typeof raw !== "object" || raw === null) return null;
  const { pid, port, startedAt, owner, supervisor, version } = raw as Record<string, unknown>;
  if (typeof port !== "number" || !Number.isInteger(port) || port < 1 || port > 65535) return null;
  if (typeof pid !== "number" || !Number.isInteger(pid) || pid <= 0) return null;
  const startedAtMs = typeof startedAt === "string" ? Date.parse(startedAt) : NaN;
  return {
    pid,
    port,
    startedAtMs: Number.isNaN(startedAtMs) ? 0 : startedAtMs,
    owner: typeof owner === "string" ? owner : undefined,
    supervisor: typeof supervisor === "string" ? supervisor : undefined,
    version: typeof version === "string" ? version : undefined,
  };
}

function joinPath(...segments: string[]): string {
  return segments.map((s) => s.replace(/[/\\]+$/, "")).join("/");
}

export function defaultDataDir(
  platform: NodeJS.Platform,
  env: Record<string, string | undefined>,
  homeDir: string,
): string {
  const override = env.BABYSITTER_DATA_DIR?.trim();
  if (override) return override;
  if (platform === "darwin") return joinPath(homeDir, "Library", "Application Support", "babysitter");
  if (platform === "win32") {
    // Windows paths keep their backslashes, so the daemon logs and shows
    // one kind of separator.
    const appData = env.APPDATA?.trim() || [homeDir.replace(/[/\\]+$/, ""), "AppData", "Roaming"].join("\\");
    return [appData.replace(/[/\\]+$/, ""), "babysitter"].join("\\");
  }
  const xdg = env.XDG_CONFIG_HOME?.trim() || joinPath(homeDir, ".config");
  return joinPath(xdg, "babysitter");
}

export function apiBaseUrl(port: number): string {
  return `http://127.0.0.1:${port}/api/v1`;
}
