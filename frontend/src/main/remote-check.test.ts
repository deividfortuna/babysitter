import { expect, test } from "vite-plus/test";
import { checkRemote } from "./remote-check";

function daemonAnswering(health: unknown): typeof fetch {
  return (async (input: string) => {
    const path = new URL(input).pathname;
    if (path.endsWith("/healthz")) return Response.json(health);
    return Response.json({});
  }) as typeof fetch;
}

test("a daemon that names itself and its version is accepted with both", async () => {
  const fetcher = daemonAnswering({ status: "ok", name: "studio", version: "1.2.3" });

  expect(await checkRemote("http://studio.local:7420", "secret", fetcher)).toEqual({
    ok: true,
    name: "studio",
    version: "1.2.3",
  });
});

test("a name that is not text gives way to the host name", async () => {
  const fetcher = daemonAnswering({ status: "ok", name: 42, version: "1.2.3" });

  expect(await checkRemote("http://studio.local:7420", "secret", fetcher)).toEqual({
    ok: true,
    name: "studio.local",
    version: "1.2.3",
  });
});

test("a server that does not answer as a babysitter daemon is refused", async () => {
  const notADaemon = [
    {},
    { status: "ok" },
    { status: "ok", version: { major: 1 } },
    { status: "down", version: "1.2.3" },
  ];

  for (const health of notADaemon) {
    expect(await checkRemote("http://studio.local:7420", "secret", daemonAnswering(health))).toEqual({
      ok: false,
      error: "No babysitter daemon answers at http://studio.local:7420.",
    });
  }
});

function daemonWithSettings(status: number): typeof fetch {
  return (async (input: string) => {
    const path = new URL(input).pathname;
    if (path.endsWith("/healthz")) return Response.json({ status: "ok", name: "studio", version: "1.2.3" });
    return Response.json({ error: { code: "x", message: "x" } }, { status });
  }) as typeof fetch;
}

test("a refused token tells the user to pair again", async () => {
  const result = await checkRemote("http://studio.local:7420", "wrong", daemonWithSettings(401));

  expect(result).toEqual({ ok: false, error: "The daemon refused the token. Run babysitter daemon pair on it again." });
});

test("a daemon that fails for another reason does not blame the token", async () => {
  const result = await checkRemote("http://studio.local:7420", "secret", daemonWithSettings(503));

  expect(result.ok).toBe(false);
  expect(!result.ok && result.error).not.toMatch(/token/);
  expect(!result.ok && result.error).toMatch(/503/);
});
