import { expect, test } from "vitest";
import { presents } from "./presenting";

const reading = { isSuccess: false, isError: false };
const known = { isSuccess: true, isError: false };
const failed = { isSuccess: false, isError: true };

test("the feed waits until the platform and the settings have both answered", () => {
  expect(presents(null, known)).toBeNull();
  expect(presents(true, reading)).toBeNull();
});

test("a platform that draws no banner leaves the screen to the daemon", () => {
  expect(presents(false, known)).toBe(false);
});

test("the app claims the screen once it knows what to draw", () => {
  expect(presents(true, known)).toBe(true);
});

test("a settings read that failed hands the screen back to the daemon", () => {
  expect(presents(true, failed)).toBe(false);
});
