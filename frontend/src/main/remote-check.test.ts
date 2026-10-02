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
  const fetcher = daemonAnswering({ ok: true, name: "studio", version: "1.2.3" });

  expect(await checkRemote("http://studio.local:7420", "secret", fetcher)).toEqual({
    ok: true,
    name: "studio",
    version: "1.2.3",
  });
});

test("a name or a version that is not text is left out", async () => {
  const fetcher = daemonAnswering({ ok: true, name: 42, version: { major: 1 } });

  expect(await checkRemote("http://studio.local:7420", "secret", fetcher)).toEqual({
    ok: true,
    name: "studio.local",
    version: "",
  });
});
