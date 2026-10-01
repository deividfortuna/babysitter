import { expect, test } from "vite-plus/test";
import { isMenuAnchor } from "./app-menu";

test("a menu anchor is a point with two finite numbers", () => {
  expect(isMenuAnchor({ x: 12, y: 38 })).toBe(true);
  expect(isMenuAnchor({ x: 12 })).toBe(false);
  expect(isMenuAnchor({ x: "12", y: 38 })).toBe(false);
  expect(isMenuAnchor({ x: Number.NaN, y: 38 })).toBe(false);
  expect(isMenuAnchor(null)).toBe(false);
});
