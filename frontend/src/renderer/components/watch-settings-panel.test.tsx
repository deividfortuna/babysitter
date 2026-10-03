import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildWatch } from "@test/fixtures";
import { chooseOption, focusOrder, renderWithProviders } from "@test/test-utils";
import { branchRuleApprovals, serveApi, type Decision } from "@test/msw";
import type { Watch } from "@/hooks/useWatches";
import { WatchDetail } from "./watch-detail";

function renderDetail(watch: Partial<Watch> = {}) {
  const w = buildWatch({ id: 42, ...watch });
  const decisions: Decision[] = [];
  serveApi({ watches: [w], watchById: { 42: w }, proposals: { 42: [] }, decisions });
  renderWithProviders(<WatchDetail id={42} enabled onNavigate={vi.fn()} onStopped={vi.fn()} onWatchPR={vi.fn()} />);
  return { decisions, user: userEvent.setup() };
}

async function openSettings(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Watch settings" }));
  return screen.getByRole("complementary", { name: "Watch settings" });
}

test("an icon button after Merge opens the panel, and the same button in the same place closes it", async () => {
  const { user } = renderDetail();

  const open = await screen.findByRole("button", { name: "Watch settings" });
  expect(screen.queryByRole("complementary", { name: "Watch settings" })).toBeNull();

  const panel = await openSettings(user);
  expect(within(panel).getByText("For octo/babysitter#12 only. They start as your defaults.")).toBeVisible();
  expect(screen.getByRole("button", { name: "Watch settings" })).toBe(open);
  expect(screen.getByRole("heading", { level: 1 })).toBeVisible();

  await user.click(open);
  expect(screen.queryByRole("complementary", { name: "Watch settings" })).toBeNull();
});

test("the panel icon comes before the activity and the watch settings in the focus order", async () => {
  const { user } = renderDetail();

  const panel = await openSettings(user);
  const toggle = screen.getByRole("button", { name: "Watch settings" });
  const header = screen.getByRole("banner");

  const order = await focusOrder(user);
  const firstAfter = order.findIndex((element) => element !== toggle && !header.contains(element));
  expect(order.some((element) => panel.contains(element))).toBe(true);
  expect(order.indexOf(toggle)).toBeGreaterThanOrEqual(0);
  expect(order.indexOf(toggle)).toBeLessThan(firstAfter);
});

test("the header ends with a no-drag space under the panel icon, so a click on the icon does not drag the window", async () => {
  renderDetail();

  await screen.findByRole("button", { name: "Watch settings" });

  const space = screen.getByRole("banner").lastElementChild;
  expect(space).toHaveAttribute("data-slot", "panel-toggle-space");
});

test("the header no longer holds the approval mode", async () => {
  renderDetail({ approvalMode: "manual" });

  await screen.findByRole("button", { name: "Watch settings" });
  expect(screen.queryByLabelText("Approval mode")).toBeNull();
  expect(screen.queryByText("approve a clean rebase")).toBeNull();
});

test("the panel holds the copy of the defaults of this watch", async () => {
  const { user } = renderDetail({
    approvalMode: "manual",
    autoApproveRebase: true,
    approvalsRequired: 3,
    mergeMethod: "squash",
  });

  const panel = await openSettings(user);
  expect(within(panel).getByLabelText("Approval mode")).toHaveTextContent("manual");
  expect(within(panel).getByRole("switch", { name: "Approve a clean rebase or merge on its own" })).toBeChecked();
  expect(within(panel).getByLabelText("Approvals before ready to merge")).toHaveValue(3);
  expect(within(panel).getByLabelText("Merge method")).toHaveTextContent("Squash");
});

test("the approvals of the watch change when the field loses focus", async () => {
  const { decisions, user } = renderDetail({ approvalsRequired: 1 });

  const panel = await openSettings(user);
  const approvals = within(panel).getByLabelText("Approvals before ready to merge");
  await user.clear(approvals);
  await user.type(approvals, "0");
  await user.tab();

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toEqual({ route: "update", watch: 42, body: { approvalsRequired: 0 } });
});

test("an empty approvals field reads the rule of the base branch again", async () => {
  const { decisions, user } = renderDetail({ approvalsRequired: 1 });

  const panel = await openSettings(user);
  const approvals = within(panel).getByLabelText("Approvals before ready to merge");
  await user.clear(approvals);
  await user.keyboard("{Enter}");

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0].body).toEqual({ approvalsRequired: null });
  await waitFor(() => expect(approvals).toHaveValue(branchRuleApprovals));
});

test("the same approvals send nothing", async () => {
  const { decisions, user } = renderDetail({ approvalsRequired: 1 });

  const panel = await openSettings(user);
  await user.click(within(panel).getByLabelText("Approvals before ready to merge"));
  await user.tab();

  expect(decisions).toHaveLength(0);
});

test("approvals that are not a whole number stay on the screen", async () => {
  const { decisions, user } = renderDetail({ approvalsRequired: 1 });

  const panel = await openSettings(user);
  const approvals = within(panel).getByLabelText("Approvals before ready to merge");
  await user.clear(approvals);
  await user.type(approvals, "-1");
  await user.tab();

  expect(within(panel).getByText("Use a whole number from 0, or leave it empty.")).toBeVisible();
  expect(approvals).toHaveAttribute("aria-invalid", "true");
  expect(decisions).toHaveLength(0);
});

test("the merge method of the watch changes", async () => {
  const { decisions, user } = renderDetail({ mergeMethod: "squash" });

  const panel = await openSettings(user);
  await chooseOption(user, within(panel).getByLabelText("Merge method"), "Rebase");

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toEqual({ route: "update", watch: 42, body: { mergeMethod: "rebase" } });
});

test("the branch update of the watch changes", async () => {
  const { decisions, user } = renderDetail({ branchUpdate: "rebase", updateOnGitHub: true });

  const panel = await openSettings(user);
  expect(within(panel).getByLabelText("Branch behind its base")).toHaveTextContent("Rebase");
  await chooseOption(user, within(panel).getByLabelText("Branch behind its base"), "Merge");
  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toEqual({ route: "update", watch: 42, body: { branchUpdate: "merge" } });

  await user.click(within(panel).getByRole("switch", { name: "Update the branch on GitHub first" }));
  await waitFor(() => expect(decisions).toHaveLength(2));
  expect(decisions[1].body).toEqual({ updateOnGitHub: false });
});

test("a Dependabot watch says the bot updates its branch", async () => {
  const { user } = renderDetail({ author: "dependabot[bot]", dependabot: true, branchUpdater: "dependabot" });

  const panel = await openSettings(user);
  expect(within(panel).getAllByText("Dependabot owns the branch, so only the bot updates it.")).toHaveLength(2);
  expect(within(panel).getByLabelText("Branch behind its base")).toBeDisabled();
  expect(within(panel).getByRole("switch", { name: "Update the branch on GitHub first" })).toBeDisabled();
});

test.each<Partial<Watch>>([
  { branchUpdater: "dependabot", author: "dependabot[bot]", dependabot: true },
  { branchUpdater: "session", provider: "self" },
])("a watch whose branch $branchUpdater updates shows GitHub off", async (watch) => {
  const { user } = renderDetail({ ...watch, updateOnGitHub: true });

  const panel = await openSettings(user);
  expect(within(panel).getByRole("switch", { name: "Update the branch on GitHub first" })).not.toBeChecked();
});

test("a self watch shows a fixed auto badge", async () => {
  const { decisions, user } = renderDetail({
    provider: "self",
    approvalMode: "auto",
    sourceDir: "/home/me/babysitter",
  });

  const panel = await openSettings(user);
  expect(within(panel).getByTitle("A watch your own coding session drives has no gate")).toHaveTextContent("auto");
  expect(within(panel).queryByLabelText("Approval mode")).toBeNull();
  expect(within(panel).queryByRole("switch", { name: "Approve a clean rebase or merge on its own" })).toBeNull();

  await chooseOption(user, within(panel).getByLabelText("Merge method"), "Merge commit");
  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0].body).toEqual({ mergeMethod: "merge" });
});

test("a stopped watch has no settings to change", async () => {
  renderDetail({ status: "stopped", stopReason: "user", stoppedAt: "2026-09-07T13:00:00Z" });

  await screen.findByText(/Here is what happened while you were away/);
  expect(screen.queryByRole("button", { name: "Watch settings" })).toBeNull();
});

test("merge when ready is saved on the watch", async () => {
  const { decisions, user } = renderDetail({ mergeWhenReady: false });

  const panel = await openSettings(user);
  const mergeWhenReady = within(panel).getByRole("switch", { name: "Merge when ready" });
  expect(mergeWhenReady).not.toBeChecked();
  expect(within(panel).getByText("The daemon merges as soon as the watch is ready to merge.")).toBeVisible();
  await user.click(mergeWhenReady);

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toEqual({ route: "update", watch: 42, body: { mergeWhenReady: true } });
});

test("a Dependabot watch says the scope of the repository turned merge when ready on", async () => {
  const { user } = renderDetail({
    dependabot: true,
    autoReason: "dependabot",
    updateType: "patch",
    mergeWhenReady: true,
  });

  const panel = await openSettings(user);
  expect(within(panel).getByRole("switch", { name: "Merge when ready" })).toBeChecked();
  expect(
    within(panel).getByText(
      "The daemon merges as soon as the watch is ready to merge. On because patch is within the scope of the repository.",
    ),
  ).toBeVisible();
});
