import { mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import path from "node:path";
import { NO_CONNECTIONS, type Connections, type RemoteConnection } from "../shared/connections";

const FILE_NAME = "connections.json";

export function connectionsPath(dataDir: string): string {
  return path.join(dataDir, FILE_NAME);
}

function isRemote(value: unknown): value is RemoteConnection {
  if (typeof value !== "object" || value === null) return false;
  const { id, name, url, token } = value as Record<string, unknown>;
  return [id, name, url, token].every((field) => typeof field === "string" && field !== "");
}

export function parseConnections(contents: string): Connections {
  let raw: unknown;
  try {
    raw = JSON.parse(contents);
  } catch {
    return NO_CONNECTIONS;
  }
  if (typeof raw !== "object" || raw === null) return NO_CONNECTIONS;
  const { activeId, remotes } = raw as Record<string, unknown>;
  const saved = Array.isArray(remotes) ? remotes.filter(isRemote) : [];
  const known = typeof activeId === "string" && saved.some((remote) => remote.id === activeId);
  return { activeId: known ? activeId : NO_CONNECTIONS.activeId, remotes: saved };
}

export function readConnections(dataDir: string): Connections {
  try {
    return parseConnections(readFileSync(connectionsPath(dataDir), "utf8"));
  } catch {
    return NO_CONNECTIONS;
  }
}

export function writeConnections(dataDir: string, connections: Connections): void {
  mkdirSync(dataDir, { recursive: true, mode: 0o750 });
  const file = connectionsPath(dataDir);
  const temporary = `${file}.tmp`;
  writeFileSync(temporary, `${JSON.stringify(connections, null, 2)}\n`, { mode: 0o600 });
  renameSync(temporary, file);
}
