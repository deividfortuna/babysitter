import { expect, test } from "vitest";
import { settingsSummary } from "./start-watch-summary";

const defaults = { agent: "Claude", model: "", approvalMode: "manual", approvals: "", mergeMethod: "" } as const;

test("names the defaults of a new watch", () => {
  expect(settingsSummary(defaults)).toBe("Claude · manual · rule of the base branch · repository default");
});

test("names the model after the agent when it is not the default", () => {
  expect(settingsSummary({ ...defaults, model: "Sonnet" })).toBe(
    "Claude Sonnet · manual · rule of the base branch · repository default",
  );
});

test("counts the approvals", () => {
  expect(settingsSummary({ ...defaults, approvals: "1" })).toBe("Claude · manual · 1 approval · repository default");
  expect(settingsSummary({ ...defaults, approvals: "2" })).toBe("Claude · manual · 2 approvals · repository default");
  expect(settingsSummary({ ...defaults, approvals: "0" })).toBe("Claude · manual · no approval · repository default");
});

test("says when the approvals are not a whole number", () => {
  expect(settingsSummary({ ...defaults, approvals: "2.5" })).toBe(
    "Claude · manual · approvals need a whole number · repository default",
  );
});

test("names the approval mode and the merge method", () => {
  expect(settingsSummary({ ...defaults, agent: "Copilot", approvalMode: "auto", mergeMethod: "merge" })).toBe(
    "Copilot · auto · rule of the base branch · merge commit",
  );
});
