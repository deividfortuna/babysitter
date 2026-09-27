import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { stubMatchMedia } from "@test/test-utils";
import { useIsMobile } from "./use-mobile";

afterEach(() => {
  vi.unstubAllGlobals();
});

test("says a narrow window is mobile", () => {
  stubMatchMedia(true);
  vi.stubGlobal("innerWidth", 500);

  const { result } = renderHook(() => useIsMobile());

  expect(result.current).toBe(true);
});

test("follows the window across the breakpoint", () => {
  const media = stubMatchMedia(false);
  vi.stubGlobal("innerWidth", 1024);
  const { result } = renderHook(() => useIsMobile());
  expect(result.current).toBe(false);

  vi.stubGlobal("innerWidth", 500);
  act(() => media.set(true));

  expect(result.current).toBe(true);
});

test("stops listening when it unmounts", () => {
  const media = stubMatchMedia(false);
  const { unmount } = renderHook(() => useIsMobile());
  expect(media.listenerCount).toBe(1);

  unmount();

  expect(media.listenerCount).toBe(0);
});
