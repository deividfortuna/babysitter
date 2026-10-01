import { describe, expect, test } from "vite-plus/test";
import {
  assertDispatchRef,
  assertStableVersion,
  baseVersion,
  channelOf,
  channelOfVersion,
  latestRelease,
  nightlyTooSoon,
  planRelease,
  stableVersionOf,
  trainVersion,
  windowsVersion,
} from "./release-channel";

const sha = "0123456789abcdef0123456789abcdef01234567";

describe("channelOf", () => {
  test("the schedule releases a nightly and a tag push releases a stable version", () => {
    expect(channelOf("schedule", undefined)).toBe("nightly");
    expect(channelOf("push", undefined)).toBe("stable");
  });

  test("a run by hand takes the channel it names, and preview when it names none", () => {
    expect(channelOf("workflow_dispatch", "stable")).toBe("stable");
    expect(channelOf("workflow_dispatch", "nightly")).toBe("nightly");
    expect(channelOf("workflow_dispatch", "")).toBe("preview");
  });

  test("an unknown channel or event is refused", () => {
    expect(() => channelOf("workflow_dispatch", "beta")).toThrow(/not stable, nightly or preview/);
    expect(() => channelOf("pull_request", undefined)).toThrow(/does not run on pull_request/);
  });
});

describe("assertDispatchRef", () => {
  test("stable and nightly start only from the default branch", () => {
    expect(() => assertDispatchRef("stable", "refs/heads/main", "main")).not.toThrow();
    expect(() => assertDispatchRef("nightly", "refs/heads/fix", "main")).toThrow(/preview channel/);
    expect(() => assertDispatchRef("stable", "refs/heads/fix", "main")).toThrow(/starts from main/);
  });

  test("a preview starts from any branch", () => {
    expect(() => assertDispatchRef("preview", "refs/heads/fix", "main")).not.toThrow();
  });
});

describe("channelOfVersion", () => {
  test("reads the channel from the version", () => {
    expect(channelOfVersion("1.2.3")).toBe("stable");
    expect(channelOfVersion("1.2.4-nightly.20260928.41")).toBe("nightly");
    expect(channelOfVersion("1.2.4-preview.20260928.42")).toBe("preview");
  });

  test("a version of no channel has none", () => {
    expect(channelOfVersion("0.1.0-alpha.5")).toBeUndefined();
    expect(channelOfVersion("1.2.4-nightly.2026.1")).toBeUndefined();
  });
});

describe("latestRelease", () => {
  const releases = [
    { tag: "v0.1.1-nightly.20260927.10", publishedAt: "2026-09-27T08:00:00Z" },
    { tag: "v0.1.1-nightly.20260928.20", publishedAt: "2026-09-28T08:00:00Z" },
    { tag: "v0.1.1-preview.20260928.21", publishedAt: "2026-09-28T09:00:00Z" },
    { tag: "v0.1.0", publishedAt: "2026-09-26T08:00:00Z" },
    { tag: "v0.1.0-alpha.5", publishedAt: "2026-09-29T08:00:00Z" },
  ];

  test("takes the release of the channel published last", () => {
    expect(latestRelease(releases, "nightly")?.tag).toBe("v0.1.1-nightly.20260928.20");
    expect(latestRelease(releases, "stable")?.tag).toBe("v0.1.0");
    expect(latestRelease(releases, "preview")?.tag).toBe("v0.1.1-preview.20260928.21");
  });

  test("gives nothing when the channel has no release", () => {
    expect(latestRelease([], "nightly")).toBeUndefined();
  });
});

test("a nightly waits six hours after the last one", () => {
  const last = { tag: "v0.1.1-nightly.20260928.20", publishedAt: "2026-09-28T08:00:00Z" };
  expect(nightlyTooSoon(last, new Date("2026-09-28T13:59:00Z"))).toBe(true);
  expect(nightlyTooSoon(last, new Date("2026-09-28T14:00:00Z"))).toBe(false);
});

describe("baseVersion", () => {
  test("is the version of package.json before the first stable release", () => {
    expect(baseVersion("0.1.0", undefined)).toBe("0.1.0");
    expect(baseVersion("0.1.0-alpha.5", undefined)).toBe("0.1.0");
  });

  test("is the next patch of the latest stable release", () => {
    expect(baseVersion("0.1.0", "v0.1.0")).toBe("0.1.1");
    expect(baseVersion("0.1.0", "v0.1.9")).toBe("0.1.10");
  });

  test("is the version of package.json when a pull request raised it above the next patch", () => {
    expect(baseVersion("0.2.0", "v0.1.4")).toBe("0.2.0");
    expect(baseVersion("1.0.0", "v0.9.9")).toBe("1.0.0");
  });
});

test("a train version carries the channel, the day and the run", () => {
  const date = new Date("2026-09-28T23:30:00Z");
  expect(trainVersion("0.1.1", "nightly", date, 41)).toBe("0.1.1-nightly.20260928.41");
  expect(trainVersion("0.1.1", "preview", date, 42)).toBe("0.1.1-preview.20260928.42");
  expect(() => trainVersion("0.1.1", "stable", date, 43)).toThrow(/no train/);
});

test("a Windows version removes dotted prerelease components", () => {
  expect(windowsVersion("0.1.0-nightly.20260928.41")).toBe("0.1.0-nightly2026092841");
  expect(windowsVersion("0.1.0")).toBe("0.1.0");
});

test("the stable version of a nightly is the version it previews", () => {
  expect(stableVersionOf("v0.1.1-nightly.20260928.41")).toBe("0.1.1");
  expect(() => stableVersionOf("v0.1.1-preview.20260928.41")).toThrow(/not a nightly tag/);
  expect(() => stableVersionOf("0.1.1-nightly.20260928.41")).toThrow(/not a nightly tag/);
});

test("a stable version has no prerelease", () => {
  expect(() => assertStableVersion("1.2.3")).not.toThrow();
  expect(() => assertStableVersion("1.2.3-beta.1")).toThrow(/X.Y.Z/);
  expect(() => assertStableVersion("v1.2.3")).toThrow(/X.Y.Z/);
});

test("a stable version must be newer than the latest stable release, so the latest release never goes back", () => {
  expect(() => assertStableVersion("0.1.0", "v0.2.0")).toThrow(/not newer than v0.2.0/);
  expect(() => assertStableVersion("0.2.0", "v0.2.0")).toThrow(/not newer than v0.2.0/);
  expect(() => assertStableVersion("0.2.1", "v0.2.0")).not.toThrow();
  expect(() => assertStableVersion("0.10.0", "v0.9.9")).not.toThrow();
});

describe("planRelease", () => {
  const previous = { tag: "v0.1.0", publishedAt: "2026-09-26T08:00:00Z" };

  test("a stable release is the latest release, with the latest feed", () => {
    expect(planRelease("stable", "0.1.1", sha, previous)).toEqual({
      channel: "stable",
      version: "0.1.1",
      tag: "v0.1.1",
      name: "Babysitter v0.1.1",
      ref: sha,
      latest: true,
      feed: "latest-mac.yml",
      previousTag: "v0.1.0",
    });
  });

  test("a nightly is not the latest release, has the nightly feed and names its commit", () => {
    const nightly = { tag: "v0.1.1-nightly.20260927.10", publishedAt: "2026-09-27T08:00:00Z" };
    expect(planRelease("nightly", "0.1.1-nightly.20260928.41", sha, nightly)).toMatchObject({
      name: "Babysitter Nightly 0.1.1-nightly.20260928.41 (0123456789ab)",
      latest: false,
      feed: "nightly-mac.yml",
      previousTag: "v0.1.1-nightly.20260927.10",
    });
  });

  test("a preview has no feed and no previous tag, and its name warns", () => {
    const preview = { tag: "v0.1.1-preview.20260927.10", publishedAt: "2026-09-27T08:00:00Z" };
    expect(planRelease("preview", "0.1.1-preview.20260928.42", sha, preview)).toMatchObject({
      name: "Babysitter Preview (maintainer test build, do not install) 0.1.1-preview.20260928.42 (0123456789ab)",
      latest: false,
      feed: "",
      previousTag: "",
    });
  });

  test("the first release of a channel has no previous tag", () => {
    expect(planRelease("nightly", "0.1.0-nightly.20260928.1", sha).previousTag).toBe("");
  });
});
