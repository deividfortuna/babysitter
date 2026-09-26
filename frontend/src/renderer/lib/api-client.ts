import createClient from "openapi-fetch";
import type { paths } from "../../api/schema";

let baseUrl: string | null = null;
let client: ReturnType<typeof createClient<paths>> | null = null;
const listeners = new Set<() => void>();

export function getApiBaseUrl(): string | null {
  return baseUrl;
}

export function setApiBaseUrl(next: string | null): void {
  const normalized = next?.replace(/\/+$/, "") ?? null;
  if (normalized === baseUrl) return;
  baseUrl = normalized;
  client = null;
  for (const listener of listeners) listener();
}

export function subscribeApiBaseUrl(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function api() {
  if (!baseUrl) throw new Error("The daemon is not available yet.");
  client ??= createClient<paths>({ baseUrl: new URL(baseUrl).origin });
  return client;
}

export function apiErrorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === "object" && "error" in error) {
    const body = (error as { error?: { message?: string } }).error;
    if (body?.message) return body.message;
  }
  if (error instanceof Error) return error.message;
  return fallback;
}
