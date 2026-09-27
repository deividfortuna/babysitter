import { expect, test } from "vitest";
import { buildProviders, buildSettings } from "@test/fixtures";
import { agentLabel, defaultLabel, overrideOf, repositoryDefaults } from "./watch-defaults";

const settings = buildSettings({
  provider: "claude",
  model: "opus",
  approvalMode: "manual",
  approvalsRequired: 2,
  mergeMethod: "rebase",
  includeExisting: true,
  keepWorktree: true,
});

test("a repository without overrides takes each setting of the daemon", () => {
  expect(repositoryDefaults(settings, undefined)).toEqual({
    provider: "claude",
    model: "opus",
    approvalMode: "manual",
    autoApproveRebase: false,
    approvalsRequired: 2,
    mergeMethod: "rebase",
    includeExisting: true,
    includeOwn: false,
    keepWorktree: true,
  });
});

test("an override of the repository beats the daemon, field by field", () => {
  const got = repositoryDefaults(settings, {
    provider: "",
    model: "",
    approvalMode: "auto",
    mergeMethod: "",
    approvalsRequired: null,
    keepWorktree: false,
  });
  expect(got).toMatchObject({
    provider: "claude",
    model: "opus",
    approvalMode: "auto",
    mergeMethod: "rebase",
    approvalsRequired: null,
    includeExisting: true,
    keepWorktree: false,
  });
});

test("the provider of the repository brings its own model, not the one of the daemon", () => {
  const got = repositoryDefaults(settings, { provider: "copilot", model: "", approvalMode: "", mergeMethod: "" });
  expect(got).toMatchObject({ provider: "copilot", model: "" });
});

test("the labels name the value the watch takes", () => {
  expect(defaultLabel("squash")).toBe("Default (squash)");
  expect(defaultLabel(undefined)).toBe("Default");
  expect(agentLabel(buildProviders(), "claude", "sonnet")).toBe("Claude Sonnet");
  expect(agentLabel(buildProviders(), "copilot", "")).toBe("Copilot");
});

test("a value equal to the daemon is no override", () => {
  expect(overrideOf(true, true)).toBeUndefined();
  expect(overrideOf(false, true)).toBe(false);
  expect(overrideOf(null, null)).toBeUndefined();
  expect(overrideOf(null, 2)).toBeNull();
  expect(overrideOf(3, 2)).toBe(3);
});
