import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildProviders, buildSettings } from "@test/fixtures";
import { serveApi } from "@test/msw";
import { chooseOption, renderWithProviders } from "@test/test-utils";
import type { Settings } from "@/hooks/useSettings";
import type { Provider } from "@/hooks/useProviders";
import { SettingsDialog } from "./settings-dialog";

function renderAgent(settings: Settings = buildSettings(), providers?: Provider[]) {
  const savedSettings: Settings[] = [];
  serveApi({ settings, savedSettings, providers });
  renderWithProviders(<SettingsDialog open category="agent" onOpenChange={vi.fn()} />);
  return { savedSettings, user: userEvent.setup() };
}

test("the approval mode is a choice of two tiles", async () => {
  renderAgent(buildSettings({ approvalMode: "manual" }));

  expect(await screen.findByRole("radiogroup", { name: "Approval mode" })).toBeVisible();
  expect(screen.getByRole("radio", { name: /Manual/ })).toBeChecked();
  expect(screen.getByRole("radio", { name: /Auto/ })).not.toBeChecked();
});

test("a tile saves its approval mode at once", async () => {
  const { savedSettings, user } = renderAgent(buildSettings({ approvalMode: "manual" }));

  await user.click(await screen.findByText("The agent pushes and posts as soon as its turn ends."));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].approvalMode).toBe("auto");
  expect(screen.getByRole("radio", { name: /Auto/ })).toBeChecked();
});

test("in auto the clean rebase is off and says why", async () => {
  renderAgent(buildSettings({ approvalMode: "auto" }));

  expect(await screen.findByRole("switch", { name: "Approve a clean rebase on its own" })).toBeDisabled();
  expect(screen.getByText("Auto approves every turn, so this has no effect.")).toBeVisible();
});

test("in manual the clean rebase tells what still asks", async () => {
  renderAgent(buildSettings({ approvalMode: "manual" }));

  expect(await screen.findByRole("switch", { name: "Approve a clean rebase on its own" })).toBeEnabled();
  expect(screen.getByText("A rebase that conflicts always asks.")).toBeVisible();
});

test("keeping the worktree is saved at once", async () => {
  const { savedSettings, user } = renderAgent();

  await user.click(await screen.findByRole("switch", { name: "Keep the worktree when a watch stops" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].keepWorktree).toBe(true);
});

test("the agent of a new watch takes a provider and one of its models", async () => {
  const { savedSettings, user } = renderAgent();

  await waitFor(() => expect(screen.getByRole("combobox", { name: "Agent" })).toHaveTextContent("Claude"));
  await chooseOption(user, screen.getByLabelText("Model"), "Sonnet");
  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0]).toMatchObject({ provider: "claude", model: "sonnet", effort: "" });

  await chooseOption(user, screen.getByRole("combobox", { name: "Agent" }), /Copilot/);
  await waitFor(() => expect(savedSettings).toHaveLength(2));
  expect(savedSettings[1]).toMatchObject({ provider: "copilot", model: "" });
  expect(screen.getByLabelText("Model")).toHaveTextContent("Provider default");

  await chooseOption(user, screen.getByLabelText("Model"), "GPT-5.3 Codex");
  await waitFor(() => expect(savedSettings).toHaveLength(3));
  expect(savedSettings[2]).toMatchObject({ provider: "copilot", model: "gpt-5.3-codex" });
});

test("the effort of a new watch is one the model takes, and a new model starts from its default", async () => {
  const { savedSettings, user } = renderAgent(buildSettings({ model: "opus", effort: "high" }));

  const effort = await screen.findByLabelText("Effort");
  await waitFor(() => expect(effort).toHaveTextContent("High"));
  await chooseOption(user, screen.getByLabelText("Model"), "Haiku");
  await waitFor(() => expect(effort).toHaveTextContent("Model default"));
  expect(effort).toBeDisabled();

  await chooseOption(user, screen.getByLabelText("Model"), "Sonnet");
  await chooseOption(user, effort, "Extra high");

  await waitFor(() => expect(savedSettings).toHaveLength(3));
  expect(savedSettings[2]).toMatchObject({ model: "sonnet", effort: "xhigh" });
});

test("the agent of a new watch does not offer a provider whose command the daemon did not find", async () => {
  const { user } = renderAgent(buildSettings(), buildProviders([{}, { available: false }]));

  await waitFor(() => expect(screen.getByRole("combobox", { name: "Agent" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "Agent" }));
  const copilot = await screen.findByRole("option", { name: /Copilot/ });
  expect(copilot).toHaveAttribute("aria-disabled", "true");
  expect(copilot).toHaveTextContent("command not found");
});

test("the page tells when the daemon cannot give its settings", async () => {
  serveApi({ settingsFail: true });
  renderWithProviders(<SettingsDialog open category="agent" onOpenChange={vi.fn()} />);

  expect(await screen.findByText("no settings")).toBeVisible();
});
