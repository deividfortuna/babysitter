export type DaemonStatus = {
  state: "starting" | "ready" | "stopped" | "error";
  step?: "environment" | "daemon";
  source?: "spawned" | "attached";
  pid?: number;
  port?: number;
  baseUrl?: string;
  message?: string;
  details?: string;
};
