import { expect, test } from "vite-plus/test";
import { buildProviders, buildSettings } from "@test/fixtures";
import { agentLabel, defaultLabel, effortLabel, effortsOf, overrideOf, repositoryDefaults } from "./watch-defaults";

const settings = buildSettings({
  provider: "claude",
  model: "opus",
  effort: "high",
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
    effort: "high",
    approvalMode: "manual",
    autoApproveRebase: false,
    approvalsRequired: 2,
    mergeMethod: "rebase",
    includeExisting: true,
    includeOwn: false,
    keepWorktree: true,
    branchUpdate: "rebase",
    updateOnGitHub: true,
  });
});

test("an override of the repository beats the daemon, field by field", () => {
  const got = repositoryDefaults(settings, {
    provider: "",
    model: "",
    effort: "",
    approvalMode: "auto",
    mergeMethod: "",
    approvalsRequired: null,
    keepWorktree: false,
    branchUpdate: "merge",
    updateOnGitHub: false,
  });
  expect(got).toMatchObject({
    provider: "claude",
    model: "opus",
    effort: "high",
    approvalMode: "auto",
    mergeMethod: "rebase",
    approvalsRequired: null,
    includeExisting: true,
    keepWorktree: false,
    branchUpdate: "merge",
    updateOnGitHub: false,
  });
});

test("the provider of the repository brings its own model, not the one of the daemon", () => {
  const got = repositoryDefaults(settings, {
    provider: "copilot",
    model: "",
    effort: "",
    approvalMode: "",
    mergeMethod: "",
    branchUpdate: "",
  });
  expect(got).toMatchObject({ provider: "copilot", model: "", effort: "" });
});

test("the labels name the value the watch takes", () => {
  expect(defaultLabel("squash")).toBe("Default (squash)");
  expect(defaultLabel(undefined)).toBe("Default");
  expect(agentLabel(buildProviders(), "claude", "sonnet")).toBe("Claude Sonnet");
  expect(agentLabel(buildProviders(), "copilot", "")).toBe("Copilot");
  expect(agentLabel(buildProviders(), "claude", "opus", "xhigh")).toBe("Claude Opus · extra high effort");
  expect(agentLabel(buildProviders(), "claude", "", "low")).toBe("Claude · low effort");
});

test("the efforts are those of the model", () => {
  const catalog = buildProviders();
  expect(effortsOf(catalog, "claude", "opus").map((item) => item.id)).toEqual([
    "low",
    "medium",
    "high",
    "xhigh",
    "max",
  ]);
  expect(effortsOf(catalog, "claude", "haiku")).toEqual([]);
  expect(effortsOf(catalog, "gemini", "")).toEqual([]);
  expect(effortLabel(catalog, "claude", "opus", "xhigh")).toBe("Extra high");
  expect(effortLabel(catalog, "claude", "opus", "")).toBe("model default");
});

test("a value equal to the daemon is no override", () => {
  expect(overrideOf(true, true)).toBeUndefined();
  expect(overrideOf(false, true)).toBe(false);
  expect(overrideOf(null, null)).toBeUndefined();
  expect(overrideOf(null, 2)).toBeNull();
  expect(overrideOf(3, 2)).toBe(3);
});
