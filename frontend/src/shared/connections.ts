export const LOCAL_CONNECTION_ID = "local";
export const DEFAULT_REMOTE_PORT = 7420;

export type DiscoveredDaemon = {
  name: string;
  host: string;
  address: string;
  port: number;
  version: string;
  url: string;
};

export type RemoteConnection = {
  id: string;
  name: string;
  url: string;
  token: string;
};

export type Connections = {
  activeId: string;
  remotes: RemoteConnection[];
};

export type PairingTarget = {
  url: string;
  token: string;
};

export type SavedRemote = Omit<RemoteConnection, "token">;

export type ConnectionList = {
  activeId: string;
  localName: string;
  remotes: SavedRemote[];
};

export type PairRequest = {
  link: string;
  token?: string;
  name?: string;
};

export type PairResult = { ok: true; connection: SavedRemote } | { ok: false; error: string };

export const NO_CONNECTIONS: Connections = { activeId: LOCAL_CONNECTION_ID, remotes: [] };

export function isLocal(id: string): boolean {
  return id === LOCAL_CONNECTION_ID;
}

export function originOf(url: string): string | null {
  try {
    const parsed = new URL(withScheme(url.trim()));
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return null;
    if (!parsed.port && parsed.protocol === "http:") parsed.port = String(DEFAULT_REMOTE_PORT);
    return parsed.origin;
  } catch {
    return null;
  }
}

function withScheme(url: string): string {
  return /^[a-z][a-z0-9+.-]*:\/\//i.test(url) ? url : `http://${url}`;
}

export function parsePairingLink(link: string, token = ""): PairingTarget | null {
  const trimmed = link.trim();
  if (!trimmed) return null;
  const origin = originOf(trimmed);
  if (!origin) return null;
  const fragment = new URL(withScheme(trimmed)).hash.replace(/^#/, "");
  const fromLink = new URLSearchParams(fragment).get("token") ?? "";
  const chosen = token.trim() || fromLink;
  return chosen ? { url: origin, token: chosen } : null;
}

export function parsePairRequest(value: unknown): PairRequest {
  const fields = typeof value === "object" && value !== null ? (value as Record<string, unknown>) : {};
  const text = (key: string) => (typeof fields[key] === "string" ? fields[key] : undefined);
  return { link: text("link") ?? "", token: text("token"), name: text("name") };
}

function plainVersion(version?: string): string {
  return version?.trim().replace(/^v/, "") ?? "";
}

export function versionsDiffer(app: string, daemon?: string): boolean {
  const ours = plainVersion(app);
  const theirs = plainVersion(daemon);
  return ours !== "" && theirs !== "" && ours !== theirs;
}

export function versionWarning(name: string, daemon: string, app: string): string {
  return `${name} runs babysitter ${daemon}, and this app is ${app}. Some views can fail until both run the same version: update the older one.`;
}

export function authorization(token?: string): Record<string, string> {
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export function apiBase(origin: string): string {
  return `${origin.replace(/\/+$/, "")}/api/v1`;
}

export function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}
