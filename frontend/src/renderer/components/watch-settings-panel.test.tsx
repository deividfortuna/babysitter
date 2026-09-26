import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { buildWatch } from "@test/fixtures";
import { chooseOption, renderWithProviders } from "@test/test-utils";
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

test("an icon button after Merge opens the panel, and the panel closes it", async () => {
  const { user } = renderDetail();

  const merge = await screen.findByRole("button", { name: "Merge" });
  const open = screen.getByRole("button", { name: "Watch settings" });
  expect(merge.compareDocumentPosition(open) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(screen.queryByRole("complementary", { name: "Watch settings" })).toBeNull();

  const panel = await openSettings(user);
  expect(within(panel).getByText("For octo/babysitter#12 only. They start as your defaults.")).toBeVisible();
  expect(screen.queryByRole("button", { name: "Watch settings" })).toBeNull();
  expect(screen.getByRole("heading", { level: 1 })).toBeVisible();

  await user.click(within(panel).getByRole("button", { name: "Close watch settings" }));
  expect(screen.queryByRole("complementary", { name: "Watch settings" })).toBeNull();
  expect(screen.getByRole("button", { name: "Watch settings" })).toBeVisible();
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
  expect(within(panel).getByRole("switch", { name: "Approve a clean rebase on its own" })).toBeChecked();
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

test("a self watch shows a fixed auto badge", async () => {
  const { decisions, user } = renderDetail({
    provider: "self",
    approvalMode: "auto",
    sourceDir: "/home/me/babysitter",
  });

  const panel = await openSettings(user);
  expect(within(panel).getByTitle("A watch your own coding session drives has no gate")).toHaveTextContent("auto");
  expect(within(panel).queryByLabelText("Approval mode")).toBeNull();
  expect(within(panel).queryByRole("switch", { name: "Approve a clean rebase on its own" })).toBeNull();

  await chooseOption(user, within(panel).getByLabelText("Merge method"), "Merge commit");
  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0].body).toEqual({ mergeMethod: "merge" });
});

test("a stopped watch has no settings to change", async () => {
  renderDetail({ status: "stopped", stopReason: "user", stoppedAt: "2026-09-07T13:00:00Z" });

  await screen.findByText(/Here is what happened while you were away/);
  expect(screen.queryByRole("button", { name: "Watch settings" })).toBeNull();
});
