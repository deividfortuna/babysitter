import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildProposal, buildProposalDetail, buildSettings, buildWatch } from "@test/fixtures";
import { chooseOption, renderWithProviders } from "@test/test-utils";
import { apiUrl, server, serveApi, type Decision } from "@test/msw";
import type { ProposalDetail } from "@/hooks/useProposals";
import type { Settings } from "@/hooks/useSettings";
import type { Watch } from "@/hooks/useWatches";
import { SettingsDialog } from "./settings-dialog";
import { StopWatchDialog } from "./stop-watch-dialog";
import { WatchDetail } from "./watch-detail";
import { WatchingView } from "./watching-view";

function renderDetail(watch: Partial<Watch>, detail: Partial<ProposalDetail> = {}) {
  const w = buildWatch({ id: 42, ...watch });
  const p = buildProposal();
  const decisions: Decision[] = [];
  serveApi({
    watches: [w],
    watchById: { 42: w },
    proposals: { 42: w.pendingProposal ? [p] : [] },
    proposalDetail: { "42/3": buildProposalDetail(detail) },
    decisions,
  });
  renderWithProviders(<WatchDetail id={42} enabled onNavigate={vi.fn()} onStopped={vi.fn()} onWatchPR={vi.fn()} />);
  return { decisions, user: userEvent.setup() };
}

async function openSettings(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Watch settings" }));
}

test("the panel switches a watch between manual and auto", async () => {
  const { decisions, user } = renderDetail({ approvalMode: "auto" });

  await openSettings(user);
  await chooseOption(user, screen.getByLabelText("Approval mode"), "manual");

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toEqual({ route: "approval", watch: 42, body: { mode: "manual" } });
});

test("a switch to auto with a proposal waiting asks first", async () => {
  const { decisions, user } = renderDetail({ approvalMode: "manual", pendingProposal: 3 });

  await openSettings(user);
  await chooseOption(user, screen.getByLabelText("Approval mode"), "auto");
  const dialog = await screen.findByRole("dialog", { name: "Switch octo/babysitter#12 to auto" });
  expect(within(dialog).getByText(/Proposal 3 waits on you, and auto means nothing waits/)).toBeVisible();
  expect(decisions).toHaveLength(0);
  await user.click(within(dialog).getByRole("button", { name: "Release and switch to auto" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toEqual({ route: "approve", watch: 42, number: 3, body: { stopAsking: true } });
  expect(await screen.findByRole("status", { name: "Decision" })).toHaveTextContent(
    "The watch runs in auto from here.",
  );
});

test("a switch to auto does not release a push whose code could not be read", async () => {
  const { decisions, user } = renderDetail(
    { approvalMode: "manual", pendingProposal: 3 },
    { commits: [], files: [], diff: "", codeError: "git log: exit status 128" },
  );
  await screen.findByText("git log: exit status 128");

  await openSettings(user);
  await chooseOption(user, screen.getByLabelText("Approval mode"), "auto");
  const dialog = await screen.findByRole("dialog", { name: "Switch octo/babysitter#12 to auto" });
  expect(within(dialog).getByRole("alert")).toHaveTextContent("git log: exit status 128");
  const release = within(dialog).getByRole("button", { name: "Release and switch to auto" });
  expect(release).toBeDisabled();
  await user.click(release);

  expect(decisions).toHaveLength(0);
});

test("a switch to auto holds the release while the proposal that waits is not loaded", async () => {
  const { decisions, user } = renderDetail({ approvalMode: "manual", pendingProposal: 3 });
  server.use(
    http.get(apiUrl("/api/v1/watches/:id/proposals"), () =>
      HttpResponse.json({ error: { code: "internal", message: "boom" } }, { status: 500 }),
    ),
  );

  await openSettings(user);
  await chooseOption(user, screen.getByLabelText("Approval mode"), "auto");
  const dialog = await screen.findByRole("dialog", { name: "Switch octo/babysitter#12 to auto" });
  const release = within(dialog).getByRole("button", { name: "Release and switch to auto" });
  expect(release).toBeDisabled();
  await user.click(release);

  expect(decisions).toHaveLength(0);
});

test("a switch to auto releases the reply as the author edited it", async () => {
  const { decisions, user } = renderDetail({ approvalMode: "manual", pendingProposal: 3 });
  const reply = await screen.findByLabelText("Reply to mhernandez");
  await user.clear(reply);
  await user.type(reply, "Moved it, thanks.");

  await openSettings(user);
  await chooseOption(user, screen.getByLabelText("Approval mode"), "auto");
  const dialog = await screen.findByRole("dialog", { name: "Switch octo/babysitter#12 to auto" });
  expect(within(dialog).getByText("2 replies posted under your account, 1 as you edited it")).toBeVisible();
  await user.click(within(dialog).getByRole("button", { name: "Release and switch to auto" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(JSON.stringify(decisions[0].body)).toContain("Moved it, thanks.");
});

test("a switch to auto keeps the reply the author dropped", async () => {
  const { decisions, user } = renderDetail({ approvalMode: "manual", pendingProposal: 3 });
  const [first] = await screen.findAllByRole("button", { name: "Drop reply" });
  await user.click(first);
  await user.click(
    within(await screen.findByRole("dialog", { name: "Drop this reply?" })).getByRole("button", { name: "Drop reply" }),
  );

  await openSettings(user);
  await chooseOption(user, screen.getByLabelText("Approval mode"), "auto");
  const dialog = await screen.findByRole("dialog", { name: "Switch octo/babysitter#12 to auto" });
  expect(within(dialog).getByText("1 reply posted under your account")).toBeVisible();
  await user.click(within(dialog).getByRole("button", { name: "Release and switch to auto" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0].body).toEqual({ drop: [5], stopAsking: true });
});

test("a self watch has no gate", async () => {
  const { user } = renderDetail({ provider: "self", approvalMode: "auto", sourceDir: "/home/me/babysitter" });

  await openSettings(user);
  expect(await screen.findByTitle("A watch your own coding session drives has no gate")).toHaveTextContent("auto");
  expect(screen.queryByLabelText("Approval mode")).toBeNull();
  expect(screen.getByText(/That session pushes and replies itself, so this watch has no approval gate./)).toBeVisible();
});

test("the agent takes no message while a proposal waits", async () => {
  renderDetail({ approvalMode: "manual", pendingProposal: 3 });

  const box = await screen.findByLabelText("Message to the agent");
  expect(box).toBeDisabled();
  expect(box).toHaveAttribute("placeholder", "The agent takes no message while a proposal waits.");
  expect(
    screen.getByText(
      "A proposal waits on you, so nothing reaches the agent until you decide. To have it try again, reject the proposal and say why.",
    ),
  ).toBeVisible();
});

test("a watch with a proposal waiting needs you in the list", async () => {
  const waiting = buildWatch({ id: 1, title: "Retry webhooks", pendingProposal: 3, approvalMode: "manual" });
  const quiet = buildWatch({ id: 2, number: 13, title: "Quiet one" });
  serveApi({ watches: [waiting, quiet] });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  const needsYou = await screen.findByRole("region", { name: "Needs you · 1" });
  const row = within(needsYou).getByRole("button", { name: /Retry webhooks/ });
  expect(within(row).getByText("approval needed")).toBeVisible();
  expect(within(needsYou).getByRole("button", { name: "Review proposal 3" })).toBeVisible();
  expect(within(needsYou).queryByText("Quiet one")).toBeNull();
});

test("a stop declines the proposal that waits", async () => {
  const watch = buildWatch({ id: 42, pendingProposal: 3, worktreeDir: "/tmp/wt" });
  serveApi({ settings: buildSettings(), proposalDetail: { "42/3": buildProposalDetail() } });

  renderWithProviders(<StopWatchDialog open onOpenChange={vi.fn()} watch={watch} onStopped={vi.fn()} />);

  const alert = await screen.findByRole("alert");
  expect(alert).toHaveTextContent("Proposal 3 is declined.");
  await waitFor(() => expect(alert).toHaveTextContent("Its 2 commits and 2 replies never go out."));
});

test("the Agent page holds the approval mode and the clean rebase", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings({ approvalMode: "manual" }), savedSettings });
  const user = userEvent.setup();

  renderWithProviders(<SettingsDialog open category="agent" onOpenChange={vi.fn()} />);

  expect(await screen.findByRole("radio", { name: /Manual/ })).toBeChecked();
  const rebase = screen.getByRole("switch", { name: "Approve a clean rebase or merge on its own" });
  expect(rebase).not.toBeChecked();
  await user.click(rebase);

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0]).toMatchObject({ approvalMode: "manual", autoApproveRebase: true });
});

test("the clean rebase means nothing in auto", async () => {
  serveApi({ settings: buildSettings({ approvalMode: "auto" }) });

  renderWithProviders(<SettingsDialog open category="agent" onOpenChange={vi.fn()} />);

  expect(await screen.findByRole("switch", { name: "Approve a clean rebase or merge on its own" })).toBeDisabled();
  expect(screen.getByText("Auto approves every turn, so this has no effect.")).toBeVisible();
});

test("the panel sets the clean rebase of one watch", async () => {
  const { decisions, user } = renderDetail({ approvalMode: "manual" });

  await openSettings(user);
  const rebase = screen.getByRole("switch", { name: "Approve a clean rebase or merge on its own" });
  expect(rebase).toBeVisible();
  expect(rebase).not.toBeChecked();
  await user.click(rebase);

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toEqual({ route: "approval", watch: 42, body: { autoApproveRebase: true } });
});

test("the panel shows the clean rebase a watch has", async () => {
  const { user } = renderDetail({ approvalMode: "manual", autoApproveRebase: true });

  await openSettings(user);
  expect(screen.getByRole("switch", { name: "Approve a clean rebase or merge on its own" })).toBeChecked();
});

test("the clean rebase of a watch in auto means nothing, so the panel turns it off", async () => {
  const { user } = renderDetail({ approvalMode: "auto" });

  await openSettings(user);
  expect(screen.getByLabelText("Approval mode")).toHaveTextContent("auto");
  expect(screen.getByRole("switch", { name: "Approve a clean rebase or merge on its own" })).toBeDisabled();
});
