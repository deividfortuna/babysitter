import { useQuery } from "@tanstack/react-query";
import { bridge } from "../lib/bridge";
import { appVersionQueryKey } from "../lib/query-keys";

export function useAppVersion(): string | undefined {
  return useQuery({ queryKey: appVersionQueryKey, queryFn: () => bridge.app.getVersion(), staleTime: Infinity }).data;
}
