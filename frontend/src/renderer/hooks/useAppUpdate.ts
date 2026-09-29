import { useCallback, useEffect, useRef, useState } from "react";
import type { UpdateSettings, UpdateStatus } from "../../shared/updates";
import { bridge } from "../lib/bridge";
import { useBridgeStatus } from "./useBridgeStatus";

export function useAppUpdate() {
  const status = useBridgeStatus<UpdateStatus | null>(bridge.updates, null);
  const check = useCallback(() => bridge.updates.check(), []);
  const download = useCallback(() => bridge.updates.download(), []);
  const install = useCallback(() => bridge.updates.install(), []);

  return { status, check, download, install };
}

export function useUpdateSettings() {
  const [settings, setSettings] = useState<UpdateSettings | null>(null);
  const current = useRef<UpdateSettings | null>(null);

  const apply = useCallback((next: UpdateSettings | null) => {
    current.current = next;
    setSettings(next);
  }, []);

  useEffect(() => {
    let active = true;
    void bridge.updates.getSettings().then((loaded) => {
      if (active) apply(loaded);
    });
    return () => {
      active = false;
    };
  }, [apply]);

  const save = useCallback(
    async (patch: Partial<UpdateSettings>) => {
      const before = current.current;
      if (before) apply({ ...before, ...patch });
      try {
        apply(await bridge.updates.setSettings(patch));
      } catch (error) {
        apply(before);
        throw error;
      }
    },
    [apply],
  );

  return { settings, save };
}
