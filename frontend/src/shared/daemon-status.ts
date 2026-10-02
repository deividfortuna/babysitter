export type DaemonConnection = {
  id: string;
  kind: "local" | "remote";
  name: string;
  url?: string;
  version?: string;
};

export type DaemonStatus = {
  state: "starting" | "ready" | "stopped" | "error";
  step?: "environment" | "daemon";
  source?: "spawned" | "attached";
  pid?: number;
  startedAtMs?: number;
  port?: number;
  baseUrl?: string;
  token?: string;
  connection?: DaemonConnection;
  message?: string;
  details?: string;
};
