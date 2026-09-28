import { expect, test, vi } from "vite-plus/test";
import { readViewedFiles, storeViewedFiles } from "./viewed-files";

test("the viewed files of a proposal come back, and only the newest proposals are kept", () => {
  vi.useFakeTimers();
  for (let n = 1; n <= 21; n++) {
    vi.setSystemTime(n * 1000);
    storeViewedFiles(`42:${n}`, { "a.go": `blob${n}` });
  }
  vi.useRealTimers();

  expect(readViewedFiles("42:21")).toEqual({ "a.go": "blob21" });
  expect(readViewedFiles("42:2")).toEqual({ "a.go": "blob2" });
  expect(readViewedFiles("42:1")).toEqual({});
});

test("a broken store reads as nothing viewed", () => {
  window.localStorage.setItem("proposal_viewed_files", "{not json");
  expect(readViewedFiles("42:1")).toEqual({});

  window.localStorage.setItem("proposal_viewed_files", JSON.stringify({ "42:1": { at: 1, files: { "a.go": 3 } } }));
  expect(readViewedFiles("42:1")).toEqual({});
});
