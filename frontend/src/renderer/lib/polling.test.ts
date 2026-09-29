import { expect, test } from "vite-plus/test";
import {
  PRESETS,
  fasterPreset,
  presetOf,
  presetSummary,
  requestsPerHour,
  roughCount,
  shareOf,
  shortInterval,
} from "./polling";

const balanced = {
  pollIntervalSeconds: 60,
  watchIntervalSeconds: 180,
  watchMaxIntervalSeconds: 900,
  checkMaxIntervalSeconds: 900,
};

test("the balanced preset holds the defaults of the daemon", () => {
  expect(presetOf(balanced)?.id).toBe("balanced");
});

test("values that differ from every preset in one interval are no preset", () => {
  expect(presetOf({ ...balanced, checkMaxIntervalSeconds: 600 })).toBeUndefined();
});

test("the presets go from the slowest to the fastest", () => {
  expect(PRESETS.map((preset) => preset.id)).toEqual(["relaxed", "balanced", "eager"]);
  expect(fasterPreset("relaxed")?.id).toBe("balanced");
  expect(fasterPreset("balanced")?.id).toBe("eager");
  expect(fasterPreset("eager")).toBeUndefined();
  expect(fasterPreset(undefined)).toBeUndefined();
});

test("an interval reads in the largest whole unit", () => {
  expect(shortInterval(30)).toBe("30s");
  expect(shortInterval(60)).toBe("1m");
  expect(shortInterval(90)).toBe("1m 30s");
  expect(shortInterval(900)).toBe("15m");
  expect(shortInterval(7200)).toBe("2h");
});

test("the summary of a preset names the repository pass, the watch poll and the quiet limit", () => {
  expect(presetSummary(balanced)).toBe("1m · 3m · 15m");
});

test("the budget counts one request for each repository pass and each watch poll", () => {
  expect(requestsPerHour(balanced, { repos: 3, watches: 7 })).toBe(3 * 60 + 7 * 20);
  expect(requestsPerHour(balanced, { repos: 0, watches: 0 })).toBe(0);
});

test("the count is rounded to a step that fits its size", () => {
  expect(roughCount(42)).toBe(40);
  expect(roughCount(323)).toBe(320);
  expect(roughCount(1234)).toBe(1250);
});

test("the share of the limit stops at the whole", () => {
  expect(shareOf(500, 5000)).toBe(0.1);
  expect(shareOf(9000, 5000)).toBe(1);
});
