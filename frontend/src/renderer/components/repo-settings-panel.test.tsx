import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { buildRepo, buildRepoConfig, buildSettings } from "@test/fixtures";
import { chooseOption, renderWithProviders } from "@test/test-utils";
import { serveApi } from "@test/msw";
import type { RepoConfig, RepoConfigUpdate } from "@/hooks/useRepos";
import { RepoSettingsPanel } from "./repo-settings-panel";

const bridge = vi.hoisted(() => ({
  dialog: { pickDirectory: vi.fn<() => Promise<string | null>>(async () => null) },
  theme: { follow: vi.fn() },
}));

vi.mock("@/lib/bridge", () => ({ bridge }));

afterEach(() => {
  vi.clearAllMocks();
});

const withCheckout = { checkoutDir: "/home/me/code/babysitter" };

function renderPanel(config: Partial<RepoConfig> = {}, more: { refusal?: { code: string; message: string } } = {}) {
  const repoConfigBodies: RepoConfigUpdate[] = [];
  serveApi({
    repoConfig: buildRepoConfig(config),
    repoConfigBodies,
    repoConfigRefusal: more.refusal,
    settings: buildSettings({ approvalMode: "manual" }),
  });
  const onClose = vi.fn();
  renderWithProviders(<RepoSettingsPanel repo={buildRepo()} onClose={onClose} />);
  return { repoConfigBodies, onClose, user: userEvent.setup() };
}

function panel() {
  return screen.getByRole("complementary", { name: "Repository settings" });
}

test("the panel shows the configuration of the repository", async () => {
  renderPanel({
    ...withCheckout,
    autoStartMine: true,
    autoStartMineSince: "2026-09-24T09:00:00Z",
    autoWatchDependabot: true,
    autoWatchDependabotSince: "2026-09-24T09:00:00Z",
    dependabotScope: "minor",
    dependabotApproval: "ask",
    dependabotLimit: 2,
  });

  expect(await screen.findByLabelText("Checkout")).toHaveValue("/home/me/code/babysitter");
  expect(screen.getByRole("switch", { name: "My pull requests" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Include drafts" })).not.toBeChecked();
  expect(screen.getByRole("switch", { name: "Dependabot" })).toBeChecked();
  expect(screen.getByText(/^Yours or assigned to you, opened from now on\. On since .+\.$/)).toBeVisible();
  expect(screen.getByText(/^Each new update gets a watch\. On since .+\.$/)).toBeVisible();
  expect(screen.getByLabelText("Merge on its own up to")).toHaveTextContent("minor");
  expect(screen.getByLabelText("Approve for me")).toHaveTextContent("ask");
  expect(
    screen.getByText("A notification asks you to approve when the build is green and the update is in scope."),
  ).toBeVisible();
  expect(screen.getByLabelText("At the same time")).toHaveValue(2);
  await waitFor(() =>
    expect(within(panel()).getByRole("button", { name: /Watches it starts/ })).toHaveTextContent("Claude · manual"),
  );
});

test("a switch saves its toggle and says since when it is on", async () => {
  const { repoConfigBodies, user } = renderPanel(withCheckout);

  const mine = await screen.findByRole("switch", { name: "My pull requests" });
  expect(screen.getByText("Yours or assigned to you, opened from now on.")).toBeVisible();
  await user.click(mine);

  await waitFor(() => expect(repoConfigBodies).toEqual([{ autoStartMine: true }]));
  await waitFor(() => expect(mine).toBeChecked());
  expect(screen.getByText(/On since/)).toBeVisible();
});

test("every toggle waits on a checkout", async () => {
  renderPanel();

  expect(await screen.findByRole("switch", { name: "My pull requests" })).toBeDisabled();
  expect(screen.getByRole("switch", { name: "Include drafts" })).toBeDisabled();
  expect(screen.getByRole("switch", { name: "Dependabot" })).toBeDisabled();
  expect(screen.getByLabelText("Checkout")).toHaveValue("");
  expect(
    screen.getByText("Auto start makes each worktree from it. It must have a remote for octo/babysitter."),
  ).toBeVisible();
  expect(screen.queryByLabelText("Merge on its own up to")).toBeNull();
});

test("a checkout the daemon refuses shows why under the field", async () => {
  const message = "/home/me/code/web has no remote for octo/babysitter";
  const { repoConfigBodies, user } = renderPanel({}, { refusal: { code: "invalid_checkout", message } });

  const checkout = await screen.findByLabelText("Checkout");
  await user.type(checkout, "/home/me/code/web");
  await user.keyboard("{Enter}");

  expect(await screen.findByText(message)).toBeVisible();
  expect(checkout).toHaveAttribute("aria-invalid", "true");
  expect(checkout).toHaveValue("/home/me/code/web");
  expect(repoConfigBodies).toEqual([{ checkoutDir: "/home/me/code/web" }]);
  expect(screen.getByRole("switch", { name: "My pull requests" })).toBeDisabled();
});

test("a folder from the picker is saved as the checkout, and the toggles open", async () => {
  bridge.dialog.pickDirectory.mockResolvedValueOnce("/home/me/code/babysitter");
  const { repoConfigBodies, user } = renderPanel();

  await user.click(await screen.findByRole("button", { name: "Choose…" }));

  await waitFor(() => expect(repoConfigBodies).toEqual([{ checkoutDir: "/home/me/code/babysitter" }]));
  await waitFor(() => expect(screen.getByRole("switch", { name: "My pull requests" })).toBeEnabled());
  expect(screen.getByLabelText("Checkout")).toHaveValue("/home/me/code/babysitter");
});

test("the help of Approve for me follows the value, and a change is saved", async () => {
  const { repoConfigBodies, user } = renderPanel(withCheckout);

  const approval = await screen.findByLabelText("Approve for me");
  expect(screen.getByText("The daemon submits no review. A missing review waits for you.")).toBeVisible();
  await chooseOption(user, approval, "green");

  await waitFor(() => expect(repoConfigBodies).toEqual([{ dependabotApproval: "green" }]));
  expect(
    await screen.findByText("Approve in your name when the build is green and the update is in scope."),
  ).toBeVisible();
});

test("the scope of the merge is saved", async () => {
  const { repoConfigBodies, user } = renderPanel(withCheckout);

  await chooseOption(user, await screen.findByLabelText("Merge on its own up to"), "major");

  await waitFor(() => expect(repoConfigBodies).toEqual([{ dependabotScope: "major" }]));
});

test("the limit of Dependabot watches takes a whole number from 1", async () => {
  const { repoConfigBodies, user } = renderPanel(withCheckout);

  const limit = await screen.findByLabelText("At the same time");
  await user.clear(limit);
  await user.type(limit, "0");
  await user.tab();
  expect(screen.getByText("Use a whole number from 1.")).toBeVisible();
  expect(limit).toHaveAttribute("aria-invalid", "true");
  expect(repoConfigBodies).toHaveLength(0);

  await user.clear(limit);
  await user.type(limit, "3");
  await user.keyboard("{Enter}");
  await waitFor(() => expect(repoConfigBodies).toEqual([{ dependabotLimit: 3 }]));
});

test("an override sends every override of the repository at once", async () => {
  const { repoConfigBodies, user } = renderPanel({
    ...withCheckout,
    overrides: { provider: "", model: "", approvalMode: "", mergeMethod: "squash", approvalsRequired: 2 },
  });

  await user.click(await screen.findByRole("button", { name: /Watches it starts/ }));
  expect(screen.getByLabelText("Approval mode")).toHaveTextContent("Daemon setting");
  expect(screen.getByLabelText("Merge method")).toHaveTextContent("Squash");
  expect(screen.getByLabelText("Approvals")).toHaveValue(2);

  await chooseOption(user, screen.getByLabelText("Approval mode"), "auto");
  await waitFor(() => expect(repoConfigBodies).toHaveLength(1));
  expect(repoConfigBodies[0]).toEqual({
    overrides: { provider: "", model: "", approvalMode: "auto", mergeMethod: "squash", approvalsRequired: 2 },
  });
  await waitFor(() =>
    expect(within(panel()).getByRole("button", { name: /Watches it starts/ })).toHaveTextContent("Claude · auto"),
  );

  await chooseOption(user, screen.getByLabelText("Approvals before ready to merge"), "Branch rule");
  await waitFor(() => expect(repoConfigBodies).toHaveLength(2));
  expect(repoConfigBodies[1].overrides?.approvalsRequired).toBeNull();

  await chooseOption(user, screen.getByLabelText("Report existing review items"), "Report them");
  await waitFor(() => expect(repoConfigBodies).toHaveLength(3));
  expect(repoConfigBodies[2].overrides).toMatchObject({ includeExisting: true, approvalsRequired: null });

  await chooseOption(user, screen.getByLabelText("Approvals before ready to merge"), "Daemon setting");
  await waitFor(() => expect(repoConfigBodies).toHaveLength(4));
  expect(repoConfigBodies[3].overrides).not.toHaveProperty("approvalsRequired");
});

test("the agent of the watches it starts takes a provider and one of its models", async () => {
  const { repoConfigBodies, user } = renderPanel(withCheckout);

  await user.click(await screen.findByRole("button", { name: /Watches it starts/ }));
  await waitFor(() => expect(screen.getByLabelText("Agent")).toBeEnabled());
  expect(screen.getByLabelText("Model")).toBeDisabled();

  await chooseOption(user, screen.getByLabelText("Agent"), /Copilot/);
  await waitFor(() => expect(repoConfigBodies).toHaveLength(1));
  expect(repoConfigBodies[0].overrides).toMatchObject({ provider: "copilot", model: "" });

  await waitFor(() => expect(screen.getByLabelText("Model")).toBeEnabled());
  await chooseOption(user, screen.getByLabelText("Model"), "GPT-5.3 Codex");
  await waitFor(() => expect(repoConfigBodies).toHaveLength(2));
  expect(repoConfigBodies[1].overrides).toMatchObject({ provider: "copilot", model: "gpt-5.3-codex" });
});

test("the close button hands the panel back", async () => {
  const { onClose, user } = renderPanel();

  await user.click(await screen.findByRole("button", { name: "Close repository settings" }));

  expect(onClose).toHaveBeenCalled();
});
