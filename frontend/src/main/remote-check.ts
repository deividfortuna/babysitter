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

async function tokenAccepted(url: string, token: string, fetcher: Fetch): Promise<boolean | null> {
  try {
    const response = await fetcher(`${apiBase(url)}/settings`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: AbortSignal.timeout(CHECK_TIMEOUT_MS),
    });
    return response.ok;
  } catch {
    return null;
  }
}

export async function checkRemote(url: string, token: string, fetcher: Fetch = fetch): Promise<RemoteCheck> {
  const answer = await health(url, fetcher);
  if (!answer) return { ok: false, error: `No babysitter daemon answers at ${url}.` };
  const accepted = await tokenAccepted(url, token, fetcher);
  if (accepted === null) return { ok: false, error: `The daemon at ${url} stopped answering.` };
  if (!accepted) return { ok: false, error: "The daemon refused the token. Run babysitter daemon pair on it again." };
  return { ok: true, name: answer.name || new URL(url).hostname, version: answer.version ?? "" };
}
