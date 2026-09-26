import { expect, test } from "vitest";
import { canvasColor, CANVAS } from "./theme";

test("a window paints in the theme the user chose, not the one of the system", () => {
  expect(canvasColor("dark", false)).toBe(CANVAS.dark);
  expect(canvasColor("light", true)).toBe(CANVAS.light);
});

test("only a preference of system asks the system scheme", () => {
  expect(canvasColor("system", true)).toBe(CANVAS.dark);
  expect(canvasColor("system", false)).toBe(CANVAS.light);
});
