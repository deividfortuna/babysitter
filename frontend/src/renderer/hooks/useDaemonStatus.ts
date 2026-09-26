import type { DaemonStatus } from "../../shared/daemon-status";
import { bridge } from "../lib/bridge";
import { setApiBaseUrl } from "../lib/api-client";
import { useBridgeStatus } from "./useBridgeStatus";

const STARTING: DaemonStatus = { state: "starting" };

function pointApiAt(status: DaemonStatus) {
  setApiBaseUrl(status.state === "ready" ? (status.baseUrl ?? null) : null);
}

export function useDaemonStatus(): DaemonStatus {
  return useBridgeStatus(bridge.daemon, STARTING, pointApiAt);
}
