import { expect, test } from "vite-plus/test";
import { paletteMode } from "./palette";

test.each([
  { query: "", mode: "places" },
  { query: "notif", mode: "places" },
  { query: " >x", mode: "places" },
  { query: ">", mode: "actions" },
  { query: "> sett", mode: "actions" },
])("reads $query as $mode", ({ query, mode }) => {
  expect(paletteMode(query)).toBe(mode);
});
