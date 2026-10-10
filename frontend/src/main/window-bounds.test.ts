import { expect, test } from "vite-plus/test";
import { centeredBounds } from "./window-bounds";

test("the window sits in the middle of the work area, on both axes", () => {
  const workArea = { x: 0, y: 25, width: 1920, height: 1055 };

  expect(centeredBounds(workArea, { width: 1320, height: 860 })).toEqual({
    x: 300,
    y: 123,
    width: 1320,
    height: 860,
  });
});

test("the window is centered on a display that does not start at the origin", () => {
  const workArea = { x: -1440, y: 200, width: 1440, height: 900 };

  expect(centeredBounds(workArea, { width: 1000, height: 600 })).toEqual({
    x: -1220,
    y: 350,
    width: 1000,
    height: 600,
  });
});

test("the window shrinks to a work area that is smaller than its preferred size", () => {
  const workArea = { x: 0, y: 25, width: 1280, height: 775 };

  expect(centeredBounds(workArea, { width: 1320, height: 860 })).toEqual({
    x: 0,
    y: 25,
    width: 1280,
    height: 775,
  });
});
