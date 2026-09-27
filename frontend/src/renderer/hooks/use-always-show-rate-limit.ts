import { useCallback, useSyncExternalStore } from "react";

const STORAGE_KEY = "always_show_rate_limit";

const listeners = new Set<() => void>();

function readAlwaysShow(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
}

export function useAlwaysShowRateLimit() {
  const alwaysShow = useSyncExternalStore(subscribe, readAlwaysShow);

  const setAlwaysShow = useCallback((on: boolean) => {
    try {
      window.localStorage.setItem(STORAGE_KEY, String(on));
    } catch {}
    listeners.forEach((notify) => notify());
  }, []);

  return { alwaysShow, setAlwaysShow };
}
