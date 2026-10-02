import createClient from "openapi-fetch";
import type { paths } from "../../api/schema";
import { authorization } from "../../shared/connections";

let baseUrl: string | null = null;
let token: string | null = null;
let client: ReturnType<typeof createClient<paths>> | null = null;
const listeners = new Set<() => void>();

export function getApiBaseUrl(): string | null {
  return baseUrl;
}

export function setApiBaseUrl(next: string | null, nextToken: string | null = null): void {
  const normalized = next?.replace(/\/+$/, "") ?? null;
  const tokenFor = normalized ? nextToken : null;
  if (normalized === baseUrl && tokenFor === token) return;
  baseUrl = normalized;
  token = tokenFor;
  client = null;
  for (const listener of listeners) listener();
}

export function subscribeApiBaseUrl(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function api() {
  if (!baseUrl) throw new Error("The daemon is not available yet.");
  client ??= createClient<paths>({ baseUrl: new URL(baseUrl).origin, headers: authorization(token ?? undefined) });
  return client;
}

export function streamUrl(path: string, params: Record<string, string> = {}): string | null {
  if (!baseUrl) return null;
  const query = new URLSearchParams(params);
  if (token) query.set("token", token);
  const search = query.toString();
  return `${baseUrl}${path}${search ? `?${search}` : ""}`;
}

export function apiErrorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === "object" && "error" in error) {
    const body = (error as { error?: { message?: string } }).error;
    if (body?.message) return body.message;
  }
  if (error instanceof Error) return error.message;
  return fallback;
}
