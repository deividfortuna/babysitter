import { expect, test } from "vite-plus/test";
import {
  defaultChannel,
  isBusy,
  isUpdateChannel,
  parseSettingsPatch,
  releaseUrl,
  resolveSettings,
  type UpdateState,
} from "./updates";

test("only stable and prerelease are channels", () => {
  expect(isUpdateChannel("stable")).toBe(true);
  expect(isUpdateChannel("prerelease")).toBe(true);
  expect(isUpdateChannel("nightly")).toBe(false);
  expect(isUpdateChannel(undefined)).toBe(false);
});

test("a stable build follows stable releases", () => {
  expect(defaultChannel("0.2.0")).toBe("stable");
});

test("a prerelease build follows prereleases, so an alpha finds the next alpha", () => {
  expect(defaultChannel("0.1.0-alpha.1")).toBe("prerelease");
});

test("a build with no saved choice downloads on its own and follows the channel of its version", () => {
  expect(resolveSettings({}, "0.1.0-alpha.1")).toEqual({ autoDownload: true, channel: "prerelease" });
  expect(resolveSettings({}, "0.2.0")).toEqual({ autoDownload: true, channel: "stable" });
});

test("a saved choice wins over the defaults", () => {
  expect(resolveSettings({ autoDownload: false, channel: "stable" }, "0.1.0-alpha.1")).toEqual({
    autoDownload: false,
    channel: "stable",
  });
});

test("a patch keeps the keys with the right type and drops the others", () => {
  expect(parseSettingsPatch({ autoDownload: false, channel: "prerelease" })).toEqual({
    autoDownload: false,
    channel: "prerelease",
  });
  expect(parseSettingsPatch({ autoDownload: "no", channel: "nightly", other: 1 })).toEqual({});
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

test("the release page of a version is its tag on GitHub", () => {
  expect(releaseUrl("0.2.0")).toBe("https://github.com/deividfortuna/babysitter/releases/tag/v0.2.0");
});
