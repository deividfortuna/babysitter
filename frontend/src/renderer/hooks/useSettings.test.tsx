import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { expect, test } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildSettings } from "@test/fixtures";
import { apiUrl, server, serveApi } from "@test/msw";
import { createQueryClientForTests } from "@test/test-utils";
import { settingsQueryKey } from "@/lib/query-keys";
import { useSettings, useWriteSettings, type Settings } from "./useSettings";

async function harness(settings: Settings, savedSettings: Settings[] = []) {
  serveApi({ settings, savedSettings });
  const queryClient = createQueryClientForTests();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  const view = renderHook(() => ({ settings: useSettings(), writer: useWriteSettings() }), { wrapper });
  await waitFor(() => expect(view.result.current.settings.data).toBeDefined());
  return { queryClient, writer: () => view.result.current.writer };
}

test("a write sends the whole settings with the change on top", async () => {
  const savedSettings: Settings[] = [];
  const { queryClient, writer } = await harness(buildSettings({ includeOwn: true }), savedSettings);

  await act(() => writer().write({ keepWorktree: true }));

  expect(savedSettings).toHaveLength(1);
  expect(savedSettings[0]).toMatchObject({ includeOwn: true, keepWorktree: true });
  expect(queryClient.getQueryData<Settings>(settingsQueryKey)?.keepWorktree).toBe(true);
});

test("two quick writes keep both changes", async () => {
  const savedSettings: Settings[] = [];
  const { writer } = await harness(buildSettings(), savedSettings);

  await act(() => Promise.all([writer().write({ includeOwn: true }), writer().write({ keepWorktree: true })]));

  expect(savedSettings).toHaveLength(2);
  expect(savedSettings[1]).toMatchObject({ includeOwn: true, keepWorktree: true });
});

test("a write the daemon refuses puts the settings back and tells why", async () => {
  const { queryClient, writer } = await harness(buildSettings({ keepWorktree: false }));
  server.use(
    http.put(apiUrl("/api/v1/settings"), () =>
      HttpResponse.json({ error: { code: "bad_request", message: "the store is read only" } }, { status: 400 }),
    ),
  );

  await act(async () => {
    await expect(writer().write({ keepWorktree: true })).rejects.toThrow("the store is read only");
  });

  await waitFor(() => expect(writer().error?.message).toBe("the store is read only"));
  expect(queryClient.getQueryData<Settings>(settingsQueryKey)?.keepWorktree).toBe(false);
});
