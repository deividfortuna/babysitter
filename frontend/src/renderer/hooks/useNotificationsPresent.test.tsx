import { renderHook, waitFor } from "@testing-library/react";
import { expect, test, vi } from "vite-plus/test";
import { bridge } from "@/lib/bridge";
import { useNotificationsPresent } from "./useNotificationsPresent";

test("the claim is unknown until the main process answers", async () => {
  let answer: (supported: boolean) => void = () => undefined;
  vi.spyOn(bridge.notifications, "supported").mockReturnValue(
    new Promise<boolean>((resolve) => {
      answer = resolve;
    }),
  );

  const { result } = renderHook(() => useNotificationsPresent());
  expect(result.current).toBeNull();

  answer(true);
  await waitFor(() => expect(result.current).toBe(true));
});

test("a probe that fails leaves the banners to the daemon", async () => {
  vi.spyOn(bridge.notifications, "supported").mockRejectedValue(new Error("no main process"));

  const { result } = renderHook(() => useNotificationsPresent());

  await waitFor(() => expect(result.current).toBe(false));
});

test("a platform that draws no banner answers false", async () => {
  vi.spyOn(bridge.notifications, "supported").mockResolvedValue(false);

  const { result } = renderHook(() => useNotificationsPresent());

  await waitFor(() => expect(result.current).toBe(false));
});
