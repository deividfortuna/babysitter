import { expect, test } from "vite-plus/test";
import {
  defaultChannel,
  isBusy,
  isRestartable,
  isUpdateChannel,
  parseSettingsPatch,
  releaseUrl,
  resolveSettings,
  type UpdateState,
} from "./updates";

test("only stable and nightly are channels", () => {
  expect(isUpdateChannel("stable")).toBe(true);
  expect(isUpdateChannel("nightly")).toBe(true);
  expect(isUpdateChannel("prerelease")).toBe(false);
  expect(isUpdateChannel(undefined)).toBe(false);
});

test("a stable build follows stable releases", () => {
  expect(defaultChannel("0.2.0")).toBe("stable");
});

test("a nightly build follows nightlies, so it finds the next nightly", () => {
  expect(defaultChannel("0.2.1-nightly.20260928.41")).toBe("nightly");
});

test("a build of any other prerelease follows stable releases", () => {
  expect(defaultChannel("0.1.0-alpha.5")).toBe("stable");
  expect(defaultChannel("0.2.1-preview.20260928.42")).toBe("stable");
});

test("a prerelease that only ends like a nightly follows stable releases, because the updater reads its first name as the channel", () => {
  expect(defaultChannel("0.2.1-rc.1-nightly.20260928.41")).toBe("stable");
  expect(defaultChannel("0.2.1-beta-nightly.20260928.41")).toBe("stable");
});

test("a build with no saved choice downloads on its own and follows the channel of its version", () => {
  expect(resolveSettings({}, "0.2.1-nightly.20260928.41")).toEqual({ autoDownload: true, channel: "nightly" });
  expect(resolveSettings({}, "0.2.0")).toEqual({ autoDownload: true, channel: "stable" });
});

test("a saved choice wins over the defaults", () => {
  expect(resolveSettings({ autoDownload: false, channel: "stable" }, "0.2.1-nightly.20260928.41")).toEqual({
    autoDownload: false,
    channel: "stable",
  });
});

test("a patch keeps the keys with the right type and drops the others", () => {
  expect(parseSettingsPatch({ autoDownload: false, channel: "nightly" })).toEqual({
    autoDownload: false,
    channel: "nightly",
  });
  expect(parseSettingsPatch({ autoDownload: "no", channel: "prerelease", other: 1 })).toEqual({});
  expect(parseSettingsPatch({ channel: "stable" })).toEqual({ channel: "stable" });
});

test("a patch that is not an object is empty", () => {
  expect(parseSettingsPatch(null)).toEqual({});
  expect(parseSettingsPatch("stable")).toEqual({});
  expect(parseSettingsPatch([true])).toEqual({});
});

test("a check, a download and an install keep the updater busy", () => {
  expect(
    ["checking", "downloading", "downloaded", "installing"].filter((state) => isBusy(state as UpdateState)),
  ).toHaveLength(4);
  expect(
    ["unsupported", "idle", "available", "not-available", "error"].some((state) => isBusy(state as UpdateState)),
  ).toBe(false);
});

test("only a downloaded update or an install in progress offers the restart", () => {
  const states: UpdateState[] = [
    "unsupported",
    "idle",
    "checking",
    "available",
    "not-available",
    "downloading",
    "downloaded",
    "installing",
    "error",
  ];
  expect(states.filter(isRestartable)).toEqual(["downloaded", "installing"]);
});

test("the release page of a version is its tag on GitHub", () => {
  expect(releaseUrl("0.2.0")).toBe("https://github.com/deividfortuna/babysitter/releases/tag/v0.2.0");
});
