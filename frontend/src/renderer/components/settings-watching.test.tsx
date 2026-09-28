import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildProviders, buildSettings } from "@test/fixtures";
import { apiUrl, server, serveApi } from "@test/msw";
import { chooseOption, renderWithProviders } from "@test/test-utils";
import type { Settings } from "@/hooks/useSettings";
import { SettingsDialog } from "./settings-dialog";

async function openWatching() {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Watching" }));
  return user;
}

test("the Watching panel shows the settings of the daemon", async () => {
  serveApi({
    settings: buildSettings({
      pollIntervalSeconds: 120,
      watchIntervalSeconds: 45,
      approvalsRequired: 2,
      mergeMethod: "rebase",
      includeOwn: true,
    }),
  });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  await openWatching();

  expect(await screen.findByLabelText("Repository poll interval")).toHaveValue(120);
  expect(screen.getByLabelText("Watch poll interval")).toHaveValue(45);
  expect(screen.getByLabelText("Approvals before ready to merge")).toHaveValue(2);
  expect(screen.getByRole("switch", { name: "Report my own comments" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Report the review items that already exist" })).not.toBeChecked();
  expect(within(screen.getByLabelText("Merge method")).getByText("Rebase")).toBeVisible();
});

test("the agent of a new watch takes a provider and one of its models", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings(), savedSettings });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openWatching();

  await waitFor(() => expect(screen.getByLabelText("Agent")).toHaveTextContent("Claude"));
  await chooseOption(user, screen.getByLabelText("Model"), "Sonnet");
  await chooseOption(user, screen.getByLabelText("Agent"), /Copilot/);
  expect(screen.getByLabelText("Model")).toHaveTextContent("Provider default");
  await chooseOption(user, screen.getByLabelText("Model"), "GPT-5.3 Codex");
  await user.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0]).toMatchObject({ provider: "copilot", model: "gpt-5.3-codex" });
});

test("the effort of a new watch is one the model takes, and a new model starts from its default", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings({ model: "opus", effort: "high" }), savedSettings });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openWatching();

  const effort = await screen.findByLabelText("Effort");
  await waitFor(() => expect(effort).toHaveTextContent("High"));
  await chooseOption(user, screen.getByLabelText("Model"), "Haiku");
  expect(effort).toHaveTextContent("Model default");
  expect(effort).toBeDisabled();

  await chooseOption(user, screen.getByLabelText("Model"), "Sonnet");
  await chooseOption(user, effort, "Extra high");
  await user.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0]).toMatchObject({ model: "sonnet", effort: "xhigh" });
});

test("the agent of a new watch does not offer a provider whose command the daemon did not find", async () => {
  serveApi({ settings: buildSettings(), providers: buildProviders([{}, { available: false }]) });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openWatching();

  await waitFor(() => expect(screen.getByLabelText("Agent")).toBeEnabled());
  await user.click(screen.getByLabelText("Agent"));
  const copilot = await screen.findByRole("option", { name: /Copilot/ });
  expect(copilot).toHaveAttribute("aria-disabled", "true");
  expect(copilot).toHaveTextContent("command not found");
});

test("a change to an interval is saved to the daemon", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings(), savedSettings });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openWatching();

  const field = await screen.findByLabelText("Watch poll interval");
  await user.clear(field);
  await user.type(field, "45");
  await user.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].watchIntervalSeconds).toBe(45);
  expect(savedSettings[0].pollIntervalSeconds).toBe(60);
});

test("the dialog closes once the daemon took the settings", async () => {
  const onOpenChange = vi.fn();
  serveApi({ settings: buildSettings(), savedSettings: [] });

  renderWithProviders(<SettingsDialog open onOpenChange={onOpenChange} />);
  const user = await openWatching();

  await user.click(await screen.findByRole("switch", { name: "Report my own comments" }));
  await user.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
});

test("a switch is saved with the rest of the panel", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings(), savedSettings });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openWatching();

  await user.click(await screen.findByRole("switch", { name: "Keep the worktree when a watch stops" }));
  await user.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].keepWorktree).toBe(true);
});

test("an empty approvals field asks for the rule of the base branch", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings({ approvalsRequired: 2 }), savedSettings });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openWatching();

  await user.clear(await screen.findByLabelText("Approvals before ready to merge"));
  await user.click(screen.getByRole("button", { name: "Save" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].approvalsRequired).toBeNull();
});

test("an interval the daemon will not take shows what the daemon answered", async () => {
  const onOpenChange = vi.fn();
  serveApi({ settings: buildSettings() });
  server.use(
    http.put(apiUrl("/api/v1/settings"), () =>
      HttpResponse.json(
        {
          error: {
            code: "bad_request",
            message: "invalid settings: the repository poll interval must be between 10s and 24h0m0s, got 2s",
          },
        },
        { status: 400 },
      ),
    ),
  );

  renderWithProviders(<SettingsDialog open onOpenChange={onOpenChange} />);
  const user = await openWatching();

  const field = await screen.findByLabelText("Repository poll interval");
  await user.clear(field);
  await user.type(field, "2");
  await user.click(screen.getByRole("button", { name: "Save" }));

  expect(await screen.findByText(/must be between 10s and 24h0m0s/i)).toBeVisible();
  expect(onOpenChange).not.toHaveBeenCalled();
});

test("a field that holds no number is refused before it is sent", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings(), savedSettings });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openWatching();

  await user.clear(await screen.findByLabelText("Repository poll interval"));
  await user.click(screen.getByRole("button", { name: "Save" }));

  expect(await screen.findByText(/whole number of seconds/i)).toBeVisible();
  expect(savedSettings).toHaveLength(0);
});

test("Save is quiet until something changes", async () => {
  serveApi({ settings: buildSettings() });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  await openWatching();

  expect(await screen.findByRole("button", { name: "Save" })).toBeDisabled();
});
