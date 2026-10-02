import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  LOCAL_CONNECTION_ID,
  type ConnectionList,
  type DiscoveredDaemon,
  type PairRequest,
  type SavedRemote,
} from "../../shared/connections";
import { bridge } from "../lib/bridge";
import { discoveryQueryKey } from "../lib/query-keys";

const EMPTY: ConnectionList = { activeId: LOCAL_CONNECTION_ID, localName: "This computer", remotes: [] };
const DISCOVERY_REFRESH_MS = 30_000;

export function useConnections(): ConnectionList {
  const [list, setList] = useState<ConnectionList>(EMPTY);
  useEffect(() => {
    let active = true;
    void bridge.connections.list().then((next) => {
      if (active) setList(next);
    });
    const off = bridge.connections.onChange(setList);
    return () => {
      active = false;
      off();
    };
  }, []);
  return list;
}

export function useUseConnection() {
  return useMutation({ mutationFn: (id: string) => bridge.connections.use(id) });
}

export function useRemoveConnection() {
  return useMutation({ mutationFn: (id: string) => bridge.connections.remove(id) });
}

export function usePairConnection() {
  return useMutation({
    mutationFn: async (request: PairRequest) => {
      const result = await bridge.connections.pair(request);
      if (!result.ok) throw new Error(result.error);
      return result.connection;
    },
  });
}

export function useDiscoveredDaemons(enabled: boolean) {
  return useQuery({
    queryKey: discoveryQueryKey,
    queryFn: () => bridge.connections.discover(),
    enabled,
    refetchInterval: DISCOVERY_REFRESH_MS,
    staleTime: 5_000,
  });
}

function sameDaemon(daemon: DiscoveredDaemon, remote: SavedRemote): boolean {
  try {
    const saved = new URL(remote.url);
    const sameHost = saved.hostname === daemon.address || saved.hostname.split(".")[0] === daemon.host;
    return sameHost && Number(saved.port) === daemon.port;
  } catch {
    return false;
  }
}

export function useUnpairedDaemons(enabled: boolean, remotes: SavedRemote[]): DiscoveredDaemon[] {
  const discovered = useDiscoveredDaemons(enabled);
  return (discovered.data ?? []).filter((daemon) => !remotes.some((remote) => sameDaemon(daemon, remote)));
}
