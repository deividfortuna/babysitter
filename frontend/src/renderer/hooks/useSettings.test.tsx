import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { expect, test } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildSettings } from "@test/fixtures";
import { apiUrl, server, serveApi } from "@test/msw";
import { createQueryClientForTests, deferred } from "@test/test-utils";
import { settingsQueryKey } from "@/lib/query-keys";
import { useSettings, useWriteSettings, type Settings } from "./useSettings";

async function harness(settings: Settings, savedSettings: Settings[] = []) {
  serveApi({ settings, savedSettings });
  const queryClient = createQueryClientForTests();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  const view = renderHook(() => ({ settings: useSettings(), write: useWriteSettings() }), { wrapper });
  await waitFor(() => expect(view.result.current.settings.data).toBeDefined());
  return {
    queryClient,
    write: (patch: Partial<Settings>) => view.result.current.write(patch),
    shown: () => view.result.current.settings.data,
  };
}

function heldSaves(refuse: (body: Settings) => boolean = () => false) {
  const bodies: Settings[] = [];
  const turns = [deferred(), deferred(), deferred()];
  server.use(
    http.put(apiUrl("/api/v1/settings"), async ({ request }) => {
      const body = (await request.json()) as Settings;
      const turn = bodies.length;
      bodies.push(body);
      await turns[turn]?.promise;
      if (refuse(body)) {
        return HttpResponse.json({ error: { code: "bad_request", message: "refused" } }, { status: 400 });
      }
      return HttpResponse.json(body);
    }),
  );
  return { bodies, turns };
}

test("a write sends the whole settings with the change on top", async () => {
  const savedSettings: Settings[] = [];
  const { queryClient, write } = await harness(buildSettings({ includeOwn: true }), savedSettings);

  await act(() => write({ keepWorktree: true }));

  expect(savedSettings).toHaveLength(1);
  expect(savedSettings[0]).toMatchObject({ includeOwn: true, keepWorktree: true });
  expect(queryClient.getQueryData<Settings>(settingsQueryKey)?.keepWorktree).toBe(true);
});

test("two quick writes keep both changes", async () => {
  const savedSettings: Settings[] = [];
  const { write } = await harness(buildSettings(), savedSettings);

  await act(() => Promise.all([write({ includeOwn: true }), write({ keepWorktree: true })]));

  expect(savedSettings).toHaveLength(2);
  expect(savedSettings[1]).toMatchObject({ includeOwn: true, keepWorktree: true });
});

test("a write the daemon refuses leaves the settings as the daemon holds them", async () => {
  const { queryClient, write, shown } = await harness(buildSettings({ keepWorktree: false }));
  server.use(
    http.put(apiUrl("/api/v1/settings"), () =>
      HttpResponse.json({ error: { code: "bad_request", message: "the store is read only" } }, { status: 400 }),
    ),
  );

  await act(async () => {
    await expect(write({ keepWorktree: true })).rejects.toThrow("the store is read only");
  });

  await waitFor(() => expect(shown()?.keepWorktree).toBe(false));
  expect(queryClient.getQueryData<Settings>(settingsQueryKey)?.keepWorktree).toBe(false);
});

test("a queued write sends the confirmed settings and its own change, not the changes queued after it", async () => {
  const { queryClient, write, shown } = await harness(buildSettings());
  const { bodies, turns } = heldSaves();

  let all: Promise<unknown> = Promise.resolve();
  act(() => {
    all = Promise.all([write({ includeOwn: true }), write({ keepWorktree: true }), write({ includeExisting: true })]);
  });
  await waitFor(() => expect(bodies).toHaveLength(1));
  await waitFor(() => expect(shown()).toMatchObject({ includeOwn: true, keepWorktree: true, includeExisting: true }));
  expect(queryClient.getQueryData<Settings>(settingsQueryKey)?.includeOwn).toBe(false);

  turns[0].resolve();
  await waitFor(() => expect(bodies).toHaveLength(2));
  expect(bodies[1]).toMatchObject({ includeOwn: true, keepWorktree: true, includeExisting: false });

  turns[1].resolve();
  turns[2].resolve();
  await act(() => all);
  expect(bodies[2]).toMatchObject({ includeOwn: true, keepWorktree: true, includeExisting: true });
});

test("a refused write in the middle drops only its own change and keeps the one still queued", async () => {
  const { write, shown } = await harness(buildSettings());
  const { bodies, turns } = heldSaves((body) => body.keepWorktree);

  let settled: Promise<PromiseSettledResult<unknown>[]> = Promise.resolve([]);
  act(() => {
    settled = Promise.allSettled([
      write({ includeOwn: true }),
      write({ keepWorktree: true }),
      write({ includeExisting: true }),
    ]);
  });
  await waitFor(() => expect(bodies).toHaveLength(1));
  turns[0].resolve();
  turns[1].resolve();
  await waitFor(() => expect(bodies).toHaveLength(3));

  await waitFor(() => expect(shown()).toMatchObject({ includeOwn: true, keepWorktree: false, includeExisting: true }));
  expect(bodies[2]).toMatchObject({ includeOwn: true, keepWorktree: false, includeExisting: true });

  turns[2].resolve();
  const results = await act(() => settled);
  expect(results.map((result) => result.status)).toEqual(["fulfilled", "rejected", "fulfilled"]);
});

test("a write after a refused one does not send the refused change again", async () => {
  const { write } = await harness(buildSettings({ keepWorktree: false }));
  const { bodies, turns } = heldSaves((body) => body.keepWorktree);
  turns.forEach((turn) => turn.resolve());

  await act(async () => {
    await expect(write({ keepWorktree: true })).rejects.toThrow("refused");
  });
  await act(() => write({ includeOwn: true }));

  expect(bodies[1]).toMatchObject({ includeOwn: true, keepWorktree: false });
});

test("a write before the settings load sends nothing and says why", async () => {
  const puts: unknown[] = [];
  server.use(
    http.get(apiUrl("/api/v1/settings"), () => new Promise<never>(() => undefined)),
    http.put(apiUrl("/api/v1/settings"), ({ request }) => {
      puts.push(request);
      return HttpResponse.json({});
    }),
  );
  const queryClient = createQueryClientForTests();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  const view = renderHook(() => useWriteSettings(), { wrapper });

  await act(async () => {
    await expect(view.result.current({ keepWorktree: true })).rejects.toThrow("The settings are not loaded yet.");
  });
  expect(puts).toHaveLength(0);
});

test("a reload while a write runs keeps the change on screen", async () => {
  const { queryClient, write, shown } = await harness(buildSettings({ includeOwn: false }));
  const { turns } = heldSaves();

  let saving: Promise<unknown> = Promise.resolve();
  act(() => {
    saving = write({ includeOwn: true });
  });
  await waitFor(() => expect(shown()?.includeOwn).toBe(true));
  await act(() => queryClient.invalidateQueries({ queryKey: settingsQueryKey }));

  expect(queryClient.getQueryData<Settings>(settingsQueryKey)?.includeOwn).toBe(false);
  expect(shown()?.includeOwn).toBe(true);
  turns[0].resolve();
  await act(() => saving);
  expect(shown()?.includeOwn).toBe(true);
});
