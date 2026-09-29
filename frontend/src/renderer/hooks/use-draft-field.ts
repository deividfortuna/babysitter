import { useCallback, useEffect, useRef, useState } from "react";

const COMMIT_AFTER_MS = 600;

type Options<T> = {
  value: T;
  format: (value: T) => string;
  parse: (text: string) => T | undefined;
  commit: (value: T) => void;
};

export type DraftField = {
  text: string;
  invalid: boolean;
  change: (text: string) => void;
  flush: () => void;
};

export function useDraftField<T>({ value, format, parse, commit }: Options<T>): DraftField {
  const [text, setText] = useState(() => format(value));
  const [synced, setSynced] = useState(value);
  if (!Object.is(value, synced)) {
    setSynced(value);
    if (!Object.is(parse(text), value)) setText(format(value));
  }

  const pending = useRef<{ timer: ReturnType<typeof setTimeout>; next: T; typedOver: T } | null>(null);
  const latest = useRef({ commit, value });
  useEffect(() => {
    latest.current = { commit, value };
  });

  const flush = useCallback(() => {
    const queued = pending.current;
    if (!queued) return;
    clearTimeout(queued.timer);
    pending.current = null;
    const { value: current, commit: commitNext } = latest.current;
    const replacedFromOutside = !Object.is(queued.typedOver, current);
    if (replacedFromOutside || Object.is(queued.next, current)) return;
    commitNext(queued.next);
  }, []);

  useEffect(() => flush, [flush]);

  const change = useCallback(
    (next: string) => {
      setText(next);
      if (pending.current) clearTimeout(pending.current.timer);
      pending.current = null;
      const parsed = parse(next);
      if (parsed === undefined) return;
      pending.current = { timer: setTimeout(flush, COMMIT_AFTER_MS), next: parsed, typedOver: latest.current.value };
    },
    [parse, flush],
  );

  return { text, invalid: parse(text) === undefined, change, flush };
}
