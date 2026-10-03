import { useMutation, useQuery } from "@tanstack/react-query";
import { useCallback, useState } from "react";
import { isOpenTarget, type OpenTarget } from "../../shared/open-in";
import { bridge } from "@/lib/bridge";
import { openInTargetsQueryKey } from "@/lib/query-keys";

const STORAGE_KEY = "open_in_editor";
const TARGETS_STALE_MS = 5 * 60_000;
const NO_TARGETS: OpenTarget[] = [];

function readStoredTarget(): OpenTarget | null {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    return isOpenTarget(stored) ? stored : null;
  } catch {
    return null;
  }
}

function storeTarget(target: OpenTarget) {
  try {
    window.localStorage.setItem(STORAGE_KEY, target);
  } catch {}
}

export function useOpenIn(enabled: boolean) {
  const targets =
    useQuery({
      queryKey: openInTargetsQueryKey,
      queryFn: () => bridge.openIn.targets(),
      enabled,
      staleTime: TARGETS_STALE_MS,
      refetchInterval: false,
    }).data ?? NO_TARGETS;
  const [stored, setStored] = useState(readStoredTarget);
  const storedIsInstalled = stored !== null && targets.includes(stored);
  const preferred = storedIsInstalled ? stored : (targets[0] ?? null);

  const setPreferred = useCallback((target: OpenTarget) => {
    setStored(target);
    storeTarget(target);
  }, []);

  return { targets, preferred, setPreferred };
}

export type OpenFolder = { watchId: number; target: OpenTarget };

export function useOpenFolder() {
  return useMutation({
    mutationFn: async ({ watchId, target }: OpenFolder) => {
      const result = await bridge.openIn.launch(watchId, target);
      if (!result.ok) throw new Error(result.error);
    },
  });
}
