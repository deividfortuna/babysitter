import { useEffect, useState } from "react";

type StatusSource<T> = {
  getStatus(): Promise<T>;
  onStatus(listener: (status: T) => void): () => void;
};

export function useBridgeStatus<T>(source: StatusSource<T>, initial: T, onApply?: (status: T) => void): T {
  const [status, setStatus] = useState<T>(initial);

  useEffect(() => {
    let active = true;
    const apply = (next: T) => {
      if (!active) return;
      onApply?.(next);
      setStatus(next);
    };
    void source.getStatus().then(apply);
    const off = source.onStatus(apply);
    return () => {
      active = false;
      off();
    };
  }, [source, onApply]);

  return status;
}
