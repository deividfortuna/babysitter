import { apiBase } from "../shared/connections";

const CHECK_TIMEOUT_MS = 4_000;

export type RemoteCheck = { ok: true; name: string; version: string } | { ok: false; error: string };

type Fetch = typeof fetch;

type Health = { name?: string; version?: string };

function text(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

async function health(url: string, fetcher: Fetch): Promise<Health | null> {
  try {
    const response = await fetcher(`${apiBase(url)}/healthz`, { signal: AbortSignal.timeout(CHECK_TIMEOUT_MS) });
    if (!response.ok) return null;
    const body: unknown = await response.json();
    if (typeof body !== "object" || body === null) return {};
    const { name, version } = body as Record<string, unknown>;
    return { name: text(name), version: text(version) };
  } catch {
    return null;
  }
}

async function settingsWithToken(url: string, token: string, fetcher: Fetch): Promise<Response | null> {
  try {
    return await fetcher(`${apiBase(url)}/settings`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: AbortSignal.timeout(CHECK_TIMEOUT_MS),
    });
  } catch {
    return null;
  }
}

export async function checkRemote(url: string, token: string, fetcher: Fetch = fetch): Promise<RemoteCheck> {
  const answer = await health(url, fetcher);
  if (!answer) return { ok: false, error: `No babysitter daemon answers at ${url}.` };
  const settings = await settingsWithToken(url, token, fetcher);
  if (!settings) return { ok: false, error: `The daemon at ${url} stopped answering.` };
  if (settings.status === 401) {
    return { ok: false, error: "The daemon refused the token. Run babysitter daemon pair on it again." };
  }
  if (!settings.ok) return { ok: false, error: `The daemon at ${url} answered with HTTP ${settings.status}.` };
  return { ok: true, name: answer.name || new URL(url).hostname, version: answer.version ?? "" };
}
