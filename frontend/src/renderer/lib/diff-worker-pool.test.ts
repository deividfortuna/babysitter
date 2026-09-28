import { renderHook } from "@testing-library/react";
import { expect, test, vi } from "vite-plus/test";
import { created } from "@test/diffs-worker";
import { useDiffWorkerPool } from "./diff-worker-pool";

test("the workers outlive a short close of the diff and stop after a while", () => {
  vi.useFakeTimers();
  const before = created.count;

  const first = renderHook(() => useDiffWorkerPool());
  const pool = first.result.current;
  first.unmount();
  vi.advanceTimersByTime(29_000);
  const again = renderHook(() => useDiffWorkerPool());

  expect(again.result.current).toBe(pool);
  expect(created.count).toBe(before + 1);

  again.unmount();
  vi.advanceTimersByTime(30_000);
  const later = renderHook(() => useDiffWorkerPool());

  expect(later.result.current).not.toBe(pool);
  expect(created.count).toBe(before + 2);
  later.unmount();
  vi.useRealTimers();
});
