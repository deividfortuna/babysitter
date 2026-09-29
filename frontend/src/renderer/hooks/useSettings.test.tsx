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

type Gate = { open: () => void; wait: Promise<void> };

function gate(): Gate {
  let open = () => undefined as void;
  const wait = new Promise<void>((resolve) => {
    open = resolve;
  });
  return { open, wait };
}

function heldSaves(refuse: (body: Settings) => boolean = () => false) {
  const bodies: Settings[] = [];
  const gates = [gate(), gate(), gate()];
  server.use(
    http.put(apiUrl("/api/v1/settings"), async ({ request }) => {
      const body = (await request.json()) as Settings;
      const turn = bodies.length;
      bodies.push(body);
      await gates[turn]?.wait;
      if (refuse(body)) {
        return HttpResponse.json({ error: { code: "bad_request", message: "refused" } }, { status: 400 });
      }
      return HttpResponse.json(body);
    }),
  );
  return { bodies, gates };
}

test("a queued write sends the confirmed settings and its own change, not the changes queued after it", async () => {
  const { queryClient, writer } = await harness(buildSettings());
  const { bodies, gates } = heldSaves();

  let all: Promise<unknown> = Promise.resolve();
  act(() => {
    all = Promise.all([
      writer().write({ includeOwn: true }),
      writer().write({ keepWorktree: true }),
      writer().write({ includeExisting: true }),
    ]);
  });
  await waitFor(() => expect(bodies).toHaveLength(1));
  await waitFor(() =>
    expect(queryClient.getQueryData<Settings>(settingsQueryKey)).toMatchObject({
      includeOwn: true,
      keepWorktree: true,
      includeExisting: true,
    }),
  );

  gates[0].open();
  await waitFor(() => expect(bodies).toHaveLength(2));
  expect(bodies[1]).toMatchObject({ includeOwn: true, keepWorktree: true, includeExisting: false });

  gates[1].open();
  gates[2].open();
  await act(() => all);
  expect(bodies[2]).toMatchObject({ includeOwn: true, keepWorktree: true, includeExisting: true });
});

test("a refused write in the middle drops only its own change and keeps the one still queued", async () => {
  const { queryClient, writer } = await harness(buildSettings());
  const { bodies, gates } = heldSaves((body) => body.keepWorktree);

  let settled: Promise<PromiseSettledResult<unknown>[]> = Promise.resolve([]);
  act(() => {
    settled = Promise.allSettled([
      writer().write({ includeOwn: true }),
      writer().write({ keepWorktree: true }),
      writer().write({ includeExisting: true }),
    ]);
  });
  await waitFor(() => expect(bodies).toHaveLength(1));
  gates[0].open();
  gates[1].open();
  await waitFor(() => expect(bodies).toHaveLength(3));

  expect(queryClient.getQueryData<Settings>(settingsQueryKey)).toMatchObject({
    includeOwn: true,
    keepWorktree: false,
    includeExisting: true,
  });
  expect(bodies[2]).toMatchObject({ includeOwn: true, keepWorktree: false, includeExisting: true });

  gates[2].open();
  const results = await act(() => settled);
  expect(results.map((result) => result.status)).toEqual(["fulfilled", "rejected", "fulfilled"]);
});

test("a write after a refused one does not send the refused change again", async () => {
  const { writer } = await harness(buildSettings({ keepWorktree: false }));
  const bodies: Settings[] = [];
  server.use(
    http.put(apiUrl("/api/v1/settings"), async ({ request }) => {
      const body = (await request.json()) as Settings;
      bodies.push(body);
      if (body.keepWorktree) {
        return HttpResponse.json({ error: { code: "bad_request", message: "refused" } }, { status: 400 });
      }
      return HttpResponse.json(body);
    }),
  );

  await act(async () => {
    await expect(writer().write({ keepWorktree: true })).rejects.toThrow("refused");
  });
  await act(() => writer().write({ includeOwn: true }));

  expect(bodies[1]).toMatchObject({ includeOwn: true, keepWorktree: false });
});
