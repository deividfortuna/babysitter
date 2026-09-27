import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, onTestFinished, test, vi } from "vitest";
import { delay, http, HttpResponse } from "msw";
import { buildActivity, buildProposal, buildProposalDetail, buildWatch } from "@test/fixtures";
import { renderWithProviders } from "@test/test-utils";
import { apiUrl, server, serveApi, type Decision } from "@test/msw";
import type { Proposal, ProposalDetail, ProposalReply } from "@/hooks/useProposals";
import type { Watch } from "@/hooks/useWatches";
import { watchesQueryKey } from "@/lib/query-keys";
import { WatchDetail } from "./watch-detail";

function renderPending({
  watch = {},
  proposal = {},
  detail = {},
  detailFailsOnce,
}: {
  watch?: Partial<Watch>;
  proposal?: Partial<Proposal>;
  detail?: Partial<ProposalDetail>;
  detailFailsOnce?: string;
} = {}) {
  const w = buildWatch({ id: 42, approvalMode: "manual", pendingProposal: 3, ...watch });
  const p = buildProposal(proposal);
  const decisions: Decision[] = [];
  const api = {
    watches: [w],
    watchById: { 42: w },
    proposals: { 42: [p] },
    proposalDetail: { "42/3": buildProposalDetail({ ...p, ...detail }) },
    decisions,
  };
  serveApi(api);
  if (detailFailsOnce) failDetailOnce(detailFailsOnce);
  const { queryClient } = renderWithProviders(
    <WatchDetail id={42} enabled onNavigate={vi.fn()} onStopped={vi.fn()} onWatchPR={vi.fn()} />,
  );
  return { api, decisions, queryClient, user: userEvent.setup() };
}

function countDetailFetches() {
  const fetches = { count: 0 };
  const listener = ({ request }: { request: Request }) => {
    if (new URL(request.url).pathname.endsWith("/watches/42/proposals/3")) fetches.count += 1;
  };
  server.events.on("request:start", listener);
  onTestFinished(() => server.events.removeListener("request:start", listener));
  return fetches;
}

async function section() {
  return screen.findByRole("region", { name: "Proposal" });
}

function viewer(panel: HTMLElement): HTMLElement {
  const diff = within(panel).getByLabelText("Diff");
  const view = diff.firstElementChild;
  if (!(view instanceof HTMLElement)) throw new Error("the diff has no viewer");
  return view;
}

function file(panel: HTMLElement, path: string): HTMLElement {
  return within(panel).getByRole("region", { name: `File ${path}` });
}

test("a proposal that waits shows what goes out", async () => {
  renderPending();
  const panel = await section();

  expect(within(panel).getByRole("heading", { name: "Proposal 3" })).toBeVisible();
  expect(within(panel).getByText("approval needed")).toBeVisible();
  expect(within(panel).getByText("head 9f3c2a1")).toBeVisible();
  expect(within(panel).getByText("work 4e7d0b8")).toBeVisible();
  expect(
    within(panel).getByText(
      "The agent finished a turn. None of it is on GitHub yet: what you approve is what goes out, under your account.",
    ),
  ).toBeVisible();
  expect(await within(panel).findByText("2 commits on the work branch")).toBeVisible();
  expect(within(panel).getByText("Move the retry into deliver")).toBeVisible();
  expect(within(panel).getByText("2 changed files")).toBeVisible();
  const diff = within(panel).getByLabelText("Diff");
  expect(diff).toHaveTextContent("return retry(ctx, send, req)");
  expect(diff).toHaveTextContent("TestDeliverRetries");
  expect(within(panel).getByText("2 replies · posted under your account · 1 in the diff")).toBeVisible();
  expect(within(diff).getByText("This retry belongs in deliver, not in the handler.")).toBeVisible();
  expect(within(diff).getByLabelText("Reply to mhernandez")).toHaveValue(
    "Moved the retry into deliver and added a test.",
  );
  expect(
    within(panel).getByText(/pushes 2 commits to feature\/notifications · lease pinned to 9f3c2a1 · posts 2 replies/),
  ).toBeVisible();
});

test("a file in the list scrolls the diff to it", async () => {
  const { user } = renderPending();
  const panel = await section();

  await user.click(await within(panel).findByRole("button", { name: /^deliver_test.go/ }));

  expect(viewer(panel)).toHaveAttribute("data-scrolled-to", "internal/webhook/deliver_test.go");
});

test("approve releases the proposal and says what went out", async () => {
  const { decisions, user } = renderPending();
  const panel = await section();

  await user.click(await within(panel).findByRole("button", { name: "Approve" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toEqual({ route: "approve", watch: 42, number: 3, body: {} });
  expect(await screen.findByRole("status", { name: "Decision" })).toHaveTextContent(
    "Proposal 3 released: 2 commits pushed to feature/notifications and 2 replies posted.",
  );
});

test("an edited reply goes out as the author wrote it", async () => {
  const { decisions, user } = renderPending();
  const panel = await section();
  const reply = await within(panel).findByLabelText("Reply to mhernandez");

  await user.clear(reply);
  await user.type(reply, "Moved it, thanks for the catch.");
  expect(within(panel).getByText("edited")).toBeVisible();
  await user.click(within(panel).getByRole("button", { name: "Approve" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0].body).toEqual({ edits: [{ replyId: 5, body: "Moved it, thanks for the catch." }] });
});

test("a reply set back to the text of the agent goes out as the agent wrote it", async () => {
  const { decisions, user } = renderPending();
  const panel = await section();
  const reply = await within(panel).findByLabelText("Reply to mhernandez");

  await user.type(reply, " Thanks!");
  await user.clear(reply);
  await user.type(reply, "Moved the retry into deliver and added a test.");
  expect(within(panel).queryByText("edited")).toBeNull();
  await user.click(within(panel).getByRole("button", { name: "Approve" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0].body).toEqual({});
});

test("an event of the stream does not fetch the code of the proposal again", async () => {
  const fetches = countDetailFetches();
  const { queryClient } = renderPending();
  const panel = await section();
  expect(await within(panel).findByText("2 commits on the work branch")).toBeVisible();

  await queryClient.invalidateQueries({ queryKey: watchesQueryKey });

  expect(fetches.count).toBe(1);
});

test("a proposal on new work shows its new code", async () => {
  const { api, queryClient } = renderPending();
  const panel = await section();
  expect(await within(panel).findByText("Move the retry into deliver")).toBeVisible();

  const rebased = buildProposal({ headSha: "7a1b2c3", workSha: "8d9e0f1", rebasedFrom: "4e7d0b8" });
  api.proposals[42] = [rebased];
  api.proposalDetail["42/3"] = buildProposalDetail({
    ...rebased,
    commits: [{ sha: "8d9e0f1", subject: "Move the retry, on the new head" }],
  });
  await queryClient.invalidateQueries({ queryKey: watchesQueryKey });

  expect(await within(panel).findByText("Move the retry, on the new head")).toBeVisible();
});

test("a dropped reply asks first and is not sent", async () => {
  const { decisions, user } = renderPending();
  const panel = await section();

  const [first] = await within(panel).findAllByRole("button", { name: "Drop reply" });
  await user.click(first);
  const dialog = await screen.findByRole("dialog", { name: "Drop this reply?" });
  expect(within(dialog).getByText(/Their comment was never marked seen/)).toBeVisible();
  await user.click(within(dialog).getByRole("button", { name: "Drop reply" }));

  expect(
    await within(panel).findByText(
      "Reply to mhernandez dropped. Nothing is posted, and the comment reaches the agent again.",
    ),
  ).toBeVisible();
  await user.click(within(panel).getByRole("button", { name: "Approve" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0].body).toEqual({ drop: [5] });
});

test("a dropped comment that answers no comment promises nothing back", async () => {
  const { user } = renderPending();
  const panel = await section();

  const [, second] = await within(panel).findAllByRole("button", { name: "Drop reply" });
  await user.click(second);
  const dialog = await screen.findByRole("dialog", { name: "Drop this reply?" });
  expect(
    within(dialog).getByText(
      "The comment on the pull request is not posted. It answers no comment, so nothing comes back to the agent.",
    ),
  ).toBeVisible();
  await user.click(within(dialog).getByRole("button", { name: "Drop reply" }));

  expect(await within(panel).findByText("Comment on the pull request dropped. Nothing is posted.")).toBeVisible();
});

test("the heading of the replies does not count a dropped one", async () => {
  const { user } = renderPending();
  const panel = await section();
  expect(await within(panel).findByText("2 replies · posted under your account · 1 in the diff")).toBeVisible();

  const [first] = await within(panel).findAllByRole("button", { name: "Drop reply" });
  await user.click(first);
  const dialog = await screen.findByRole("dialog", { name: "Drop this reply?" });
  await user.click(within(dialog).getByRole("button", { name: "Drop reply" }));

  expect(await within(panel).findByText("1 reply · posted under your account · 1 dropped")).toBeVisible();
});

test("a commit the author kept off before is marked", async () => {
  renderPending({
    detail: {
      commits: [
        { sha: "e1197bcaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", subject: "Strip punctuation in slug", heldBack: true },
        { sha: "472a1caaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", subject: "Guard null in capitalize" },
      ],
    },
  });
  const panel = await section();

  const commit = (await within(panel).findByText("Strip punctuation in slug")).parentElement!;
  expect(within(commit).getByText("kept off before")).toBeVisible();
  expect(within(panel).queryAllByText("kept off before")).toHaveLength(1);
  expect(
    within(panel).getByText(
      "1 commit here is one you kept off the pull request in an earlier decision. Approving pushes it with the rest.",
    ),
  ).toBeVisible();
});

test("a dropped reply comes back with undo", async () => {
  const { user } = renderPending();
  const panel = await section();

  const [first] = await within(panel).findAllByRole("button", { name: "Drop reply" });
  await user.click(first);
  await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Drop reply" }));
  await user.click(await within(panel).findByRole("button", { name: "Undo" }));

  expect(within(panel).getByLabelText("Reply to mhernandez")).toBeVisible();
});

test("the push is rejected after a warning, and the replies still go out", async () => {
  const { decisions, user } = renderPending();
  const panel = await section();

  await user.click(await within(panel).findByRole("button", { name: "Reject push" }));
  const dialog = await screen.findByRole("dialog", { name: "Reject the push of proposal 3?" });
  expect(within(dialog).getByRole("alert")).toHaveTextContent("The replies still go out, and they describe this code.");
  await user.click(within(dialog).getByRole("button", { name: "Reject push" }));

  expect(await within(panel).findByText(/Push rejected. Approving posts the replies only/)).toBeVisible();
  expect(within(panel).getByText(/posts 2 replies · pushes nothing/)).toBeVisible();
  await user.click(within(panel).getByRole("button", { name: "Approve" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0].body).toEqual({ rejectPush: true });
});

test("the push comes back with restore", async () => {
  const { user } = renderPending();
  const panel = await section();

  await user.click(await within(panel).findByRole("button", { name: "Reject push" }));
  await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Reject push" }));
  await user.click(await within(panel).findByRole("button", { name: "Restore the push" }));

  expect(within(panel).getByRole("button", { name: "Reject push" })).toBeVisible();
});

test("a rejection takes a reason, and discards the commits only when asked", async () => {
  const { decisions, user } = renderPending();
  const panel = await section();

  await user.click(await within(panel).findByRole("button", { name: "Reject proposal" }));
  const dialog = await screen.findByRole("dialog", { name: "Reject proposal 3 of octo/babysitter#12" });
  const discard = within(dialog).getByRole("checkbox", { name: "Discard the commits" });
  expect(discard).not.toBeChecked();
  expect(within(dialog).getByText(/The work branch goes back to/)).toHaveTextContent("9f3c2a1");
  await user.type(within(dialog).getByLabelText("Reason (optional)"), "use a table test");
  await user.click(discard);
  await user.click(within(dialog).getByRole("button", { name: "Reject proposal" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toEqual({
    route: "reject",
    watch: 42,
    number: 3,
    body: { reason: "use a table test", discard: true },
  });
  expect(await screen.findByRole("status", { name: "Decision" })).toHaveTextContent(
    "Proposal 3 rejected. Nothing was pushed or posted",
  );
});

test("approve and stop asking names what goes out and switches the watch to auto", async () => {
  const { decisions, user } = renderPending();
  const panel = await section();

  await user.click(await within(panel).findByRole("button", { name: "Approve and stop asking" }));
  const dialog = await screen.findByRole("dialog", { name: "Approve and stop asking on octo/babysitter#12" });
  expect(within(dialog).getByText("What goes out now")).toBeVisible();
  expect(within(dialog).getByText(/2 commits pushed to/)).toBeVisible();
  expect(within(dialog).getByText("2 replies posted under your account")).toBeVisible();
  await user.click(within(dialog).getByRole("button", { name: "Approve and switch to auto" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0].body).toEqual({ stopAsking: true });
  expect(await screen.findByRole("status", { name: "Decision" })).toHaveTextContent(
    "The watch runs in auto from here.",
  );
});

test("a proposal the daemon rebased says so", async () => {
  renderPending({
    proposal: {
      rebasedFrom: "4e7d0b8bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      workSha: "c81f7e2eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
    },
  });
  const panel = await section();

  expect(within(panel).getByText("rebased")).toBeVisible();
  expect(within(panel).getByText("work c81f7e2, was 4e7d0b8")).toBeVisible();
  expect(
    within(panel).getByText(
      "The pull request branch moved while this waited, and the daemon rebased the work onto it without conflicts. It is the change you were reading, on new commits, and a reply that named an old commit now names the new one.",
    ),
  ).toBeVisible();
});

test("a Dependabot proposal carries replies only", async () => {
  renderPending({
    watch: { author: "dependabot[bot]", dependabot: true },
    proposal: { hasPush: false, replies: (buildProposal().replies ?? []).slice(1) },
    detail: { commits: [], files: [], diff: "" },
  });
  const panel = await section();

  expect(
    within(panel).getByText(
      "The agent answered a comment. Dependabot owns this branch, so the proposal carries replies only: the daemon pushes nothing and rebases nothing.",
    ),
  ).toBeVisible();
  expect(await within(panel).findByRole("button", { name: "Approve and post" })).toBeVisible();
  expect(within(panel).queryByRole("button", { name: "Reject push" })).toBeNull();
  expect(within(panel).queryByText(/work /)).toBeNull();
  expect(within(panel).getByText("1 reply · posted under your account")).toBeVisible();
});

test("a person whose login starts with dependabot gets the push of a person", async () => {
  renderPending({ watch: { author: "dependabot-fan", dependabot: false } });
  const panel = await section();

  expect(
    within(panel).getByText(
      "The agent finished a turn. None of it is on GitHub yet: what you approve is what goes out, under your account.",
    ),
  ).toBeVisible();
  expect(within(panel).queryByText(/Dependabot owns this branch/)).toBeNull();
  expect(await within(panel).findByRole("button", { name: "Reject push" })).toBeVisible();
});

test("a push that failed offers a retry", async () => {
  const { decisions, user } = renderPending({
    watch: { pendingProposal: undefined },
    proposal: {
      status: "failed",
      approvedAt: "2026-01-01T00:06:00Z",
      error: "push proposal 3: git could not read a credential for github.com",
    },
  });
  const panel = await section();

  expect(within(panel).getByText("push failed")).toBeVisible();
  expect(within(panel).getByRole("alert")).toHaveTextContent("The push did not go out, so nothing was posted either.");
  expect(within(panel).getByRole("alert")).toHaveTextContent("git could not read a credential for github.com");
  await user.click(within(panel).getByRole("button", { name: "Retry the push" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toMatchObject({ route: "retry", watch: 42, number: 3 });
});

test("a rebase that conflicts is with the agent, and asks nothing of the author", async () => {
  renderPending({
    watch: { pendingProposal: undefined },
    proposal: {
      status: "failed",
      error: "rebase proposal 3: the rebase onto 1d8e4f2 conflicts in internal/webhook/deliver.go",
    },
  });
  const panel = await section();

  expect(within(panel).getByText("rebase conflicts")).toBeVisible();
  expect(within(panel).getByText(/The daemon aborted the rebase and handed it to the agent/)).toBeVisible();
  expect(within(panel).queryByRole("button")).toBeNull();
});

test("a rebase that conflicts in auto says the resolution goes out on its own", async () => {
  renderPending({
    watch: { pendingProposal: undefined, approvalMode: "auto" },
    proposal: {
      status: "failed",
      error: "push proposal 3: the rebase onto 1d8e4f2 conflicts in internal/webhook/deliver.go",
    },
  });
  const panel = await section();

  expect(within(panel).getByText(/the daemon pushes its resolution when the turn of the agent ends/)).toBeVisible();
  expect(within(panel).queryByText(/asks you/)).toBeNull();
});

test("the row of a rebase that conflicts offers no retry", async () => {
  const w = buildWatch({ id: 42, approvalMode: "manual" });
  const failed = buildProposal({
    status: "failed",
    error: "rebase proposal 3: the rebase onto 1d8e4f2 conflicts in x.go",
  });
  serveApi({
    watches: [w],
    watchById: { 42: w },
    proposals: { 42: [failed] },
    proposalDetail: { "42/3": buildProposalDetail(failed) },
    watchActivity: {
      42: [
        buildActivity({
          id: 50,
          kind: "agent_failed",
          summary:
            "could not rebase proposal 3: the rebase onto 1d8e4f2 conflicts in x.go; the agent resolves it in its next turn",
          payload: { proposal: 3, by: "daemon" },
        }),
      ],
    },
  });
  renderWithProviders(<WatchDetail id={42} enabled onNavigate={vi.fn()} onStopped={vi.fn()} onWatchPR={vi.fn()} />);

  expect(await within(await section()).findByText("rebase conflicts")).toBeVisible();
  const row = (await screen.findByText(/could not rebase proposal 3/)).closest("div.border-b") as HTMLElement;
  expect(within(row).queryByRole("button", { name: "Retry" })).toBeNull();
});

test("a retry that goes out leaves its line", async () => {
  const { user } = renderPending({
    watch: { pendingProposal: undefined },
    proposal: { status: "failed", approvedAt: "2026-01-01T00:06:00Z", error: "push proposal 3: no credential" },
  });
  const panel = await section();

  await user.click(within(panel).getByRole("button", { name: "Retry the push" }));

  expect(await screen.findByRole("status", { name: "Decision" })).toHaveTextContent(
    "Proposal 3 released: 2 commits pushed to feature/notifications and 2 replies posted.",
  );
});

test("a failed release has a retry in the timeline", async () => {
  const w = buildWatch({ id: 42 });
  const failed = buildProposal({ status: "failed", error: "push proposal 3: no credential" });
  const decisions: Decision[] = [];
  serveApi({
    watches: [w],
    watchById: { 42: w },
    proposals: { 42: [failed] },
    proposalDetail: { "42/3": buildProposalDetail(failed) },
    watchActivity: {
      42: [
        buildActivity({
          id: 50,
          kind: "agent_failed",
          summary: "could not push proposal 3: no credential; retry with `babysitter watch retry 42 3`",
          payload: { proposal: 3, retry: "babysitter watch retry 42 3", by: "daemon" },
        }),
      ],
    },
    decisions,
  });
  const user = userEvent.setup();
  renderWithProviders(<WatchDetail id={42} enabled onNavigate={vi.fn()} onStopped={vi.fn()} onWatchPR={vi.fn()} />);

  const row = (await screen.findByText(/could not push proposal 3/)).closest("div.border-b") as HTMLElement;
  await user.click(await within(row).findByRole("button", { name: "Retry" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toMatchObject({ route: "retry", watch: 42, number: 3 });
});

function renderLive(first: Proposal) {
  const w = buildWatch({ id: 42, approvalMode: "manual", pendingProposal: first.number });
  const listed = [first];
  const details = { [`42/${first.number}`]: buildProposalDetail(first) };
  serveApi({ watches: [w], watchById: { 42: w }, proposals: { 42: listed }, proposalDetail: details });
  const view = renderWithProviders(
    <WatchDetail id={42} enabled onNavigate={vi.fn()} onStopped={vi.fn()} onWatchPR={vi.fn()} />,
  );
  const refetch = () => view.queryClient.invalidateQueries({ queryKey: ["watches"] });
  const decide = (route: "approve" | "reject", status: Proposal["status"]) =>
    server.use(
      http.post(apiUrl(`/api/v1/watches/:id/proposals/:number/${route}`), async ({ params }) => {
        const number = Number(params.number);
        const index = listed.findIndex((p) => p.number === number);
        listed[index] = { ...listed[index], status };
        await refetch();
        await delay(50);
        return HttpResponse.json(listed[index]);
      }),
    );
  const offer = async (next: Proposal) => {
    listed.unshift(next);
    details[`42/${next.number}`] = buildProposalDetail(next);
    await refetch();
  };
  return { decide, offer, user: userEvent.setup() };
}

test("the line of a release shows when the proposals change before the approval answers", async () => {
  const { decide, user } = renderLive(buildProposal());
  decide("approve", "released");
  const panel = await section();

  await user.click(await within(panel).findByRole("button", { name: "Approve" }));

  expect(await screen.findByRole("status", { name: "Decision" })).toHaveTextContent("Proposal 3 released");
});

test("a rejection never shows the line of an older release", async () => {
  const { decide, offer, user } = renderLive(buildProposal());
  await user.click(await within(await section()).findByRole("button", { name: "Approve" }));
  expect(await screen.findByRole("status", { name: "Decision" })).toHaveTextContent("Proposal 3 released");

  await offer(buildProposal({ number: 4 }));
  expect(await screen.findByRole("heading", { name: "Proposal 4" })).toBeVisible();
  decide("reject", "rejected");
  await user.click(within(await section()).getByRole("button", { name: "Reject proposal" }));
  await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Reject proposal" }));

  await waitFor(() => expect(screen.queryByRole("heading", { name: "Proposal 4" })).toBeNull());
  expect(screen.queryByText(/Proposal 3 released/)).toBeNull();
});

test("the refusal of a reject does not stay on the next proposal", async () => {
  const refusal =
    "the proposal does not wait on a decision: part of proposal 4 is on GitHub, its commits are on feat/retry; retry it to send the rest";
  const { offer, user } = renderLive(
    buildProposal({
      number: 4,
      status: "failed",
      approvedAt: "2026-01-01T00:06:00Z",
      error: "post the replies of proposal 4: the comment has no review thread",
    }),
  );
  server.use(
    http.post(apiUrl("/api/v1/watches/:id/proposals/:number/reject"), () =>
      HttpResponse.json({ error: { code: "not_pending", message: refusal } }, { status: 409 }),
    ),
  );
  await user.click(within(await section()).getByRole("button", { name: "Reject proposal" }));
  await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Reject proposal" }));
  expect(await screen.findByText(refusal)).toBeVisible();
  await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));

  await offer(buildProposal({ number: 5 }));

  expect(await screen.findByRole("heading", { name: "Proposal 5" })).toBeVisible();
  expect(within(await section()).queryByText(refusal)).toBeNull();
});

test("the push warning counts one commit in the singular", async () => {
  const { user } = renderPending({
    detail: { commits: [{ sha: "3b1e9c4cccccccccccccccccccccccccccccccc", subject: "Move the retry into deliver" }] },
  });
  const panel = await section();
  await within(panel).findByText("1 commit on the work branch");

  await user.click(within(panel).getByRole("button", { name: "Reject push" }));

  expect(await screen.findByText(/The 1 commit stays on the work branch/)).toBeVisible();
});

test("the push warning speaks of one reply as one", async () => {
  const one = (buildProposal().replies ?? []).slice(0, 1);
  const { user } = renderPending({ proposal: { replies: one }, detail: { replies: one } });
  const panel = await section();
  await within(panel).findByText("2 commits on the work branch");

  await user.click(within(panel).getByRole("button", { name: "Reject push" }));

  const alert = within(await screen.findByRole("dialog")).getByRole("alert");
  expect(alert).toHaveTextContent("The reply still goes out, and it describes this code.");
  expect(alert).toHaveTextContent(
    "It may name 3b1e9c4 and 7a20d55, which the pull request will not have. Edit or drop it before you approve.",
  );
});

function failDetailOnce(message: string) {
  server.use(
    http.get(
      apiUrl("/api/v1/watches/:id/proposals/:number"),
      () => HttpResponse.json({ error: { code: "internal", message } }, { status: 500 }),
      {
        once: true,
      },
    ),
  );
}

test("a preview that did not load shows the error and holds the approval until it is read again", async () => {
  const { user } = renderPending({ detailFailsOnce: "the worktree is gone" });
  const panel = await section();

  expect(await within(panel).findByRole("alert")).toHaveTextContent("the worktree is gone");
  expect(within(panel).getByRole("button", { name: "Approve" })).toBeDisabled();
  expect(within(panel).getByRole("button", { name: "Approve and stop asking" })).toBeDisabled();
  expect(within(panel).queryByText(/0 commits/)).toBeNull();
  expect(within(panel).getByText(/pushes commits not read yet to feature\/notifications/)).toBeVisible();

  await user.click(within(panel).getByRole("button", { name: "Read the code again" }));

  expect(await within(panel).findByText("2 commits on the work branch")).toBeVisible();
  expect(within(panel).queryByRole("alert")).toBeNull();
  expect(within(panel).getByRole("button", { name: "Approve" })).toBeEnabled();
  expect(within(panel).getByRole("button", { name: "Approve and stop asking" })).toBeEnabled();
  expect(within(panel).getByText(/pushes 2 commits to feature\/notifications/)).toBeVisible();
});

test("code the daemon could not read is an error, not zero changes, and a retry on the same work replaces it", async () => {
  const fetches = countDetailFetches();
  const { api, user } = renderPending({
    detail: { commits: [], files: [], diff: "", codeError: "git log: exit status 128" },
  });
  const panel = await section();

  expect(await within(panel).findByRole("alert")).toHaveTextContent("git log: exit status 128");
  expect(within(panel).queryByText("0 commits on the work branch")).toBeNull();
  expect(within(panel).queryByText(/0 changed files/)).toBeNull();
  expect(within(panel).getByRole("button", { name: "Approve" })).toBeDisabled();
  expect(within(panel).getByRole("button", { name: "Approve and stop asking" })).toBeDisabled();

  api.proposalDetail["42/3"] = buildProposalDetail();
  await user.click(within(panel).getByRole("button", { name: "Read the code again" }));

  expect(await within(panel).findByText("2 commits on the work branch")).toBeVisible();
  expect(fetches.count).toBe(2);
  expect(within(panel).getByRole("button", { name: "Approve" })).toBeEnabled();
});

test("a proposal with replies only is approved without its preview", async () => {
  const { decisions, user } = renderPending({ proposal: { hasPush: false }, detailFailsOnce: "the worktree is gone" });
  const panel = await section();

  await user.click(await within(panel).findByRole("button", { name: "Approve and post" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0]).toMatchObject({ route: "approve", number: 3 });
});

test("rejecting the push frees the approval while the code is not read", async () => {
  const { decisions, user } = renderPending({ detailFailsOnce: "the worktree is gone" });
  const panel = await section();
  await within(panel).findByRole("alert");

  await user.click(within(panel).getByRole("button", { name: "Reject push" }));
  const dialog = await screen.findByRole("dialog", { name: "Reject the push of proposal 3?" });
  expect(dialog).toHaveTextContent("The commits stay on the work branch");
  expect(dialog).not.toHaveTextContent("0 commits");
  await user.click(within(dialog).getByRole("button", { name: "Reject push" }));

  expect(within(panel).getByRole("button", { name: "Approve" })).toBeEnabled();
  await user.click(within(panel).getByRole("button", { name: "Approve" }));

  await waitFor(() => expect(decisions).toHaveLength(1));
  expect(decisions[0].body).toEqual({ rejectPush: true });
});

test("an open stop asking dialog holds the release when the work moves to code that cannot be read", async () => {
  const { api, decisions, queryClient, user } = renderPending();
  const panel = await section();
  await within(panel).findByText("2 commits on the work branch");
  await user.click(within(panel).getByRole("button", { name: "Approve and stop asking" }));
  const dialog = await screen.findByRole("dialog", { name: "Approve and stop asking on octo/babysitter#12" });
  expect(within(dialog).getByRole("button", { name: "Approve and switch to auto" })).toBeEnabled();

  api.proposals[42] = [buildProposal({ workSha: "7c1d2e3ccccccccccccccccccccccccccccccccc" })];
  api.proposalDetail["42/3"] = buildProposalDetail({
    commits: [],
    files: [],
    diff: "",
    codeError: "git log: exit status 128",
  });
  await queryClient.invalidateQueries({ queryKey: watchesQueryKey });

  expect(await within(dialog).findByText("git log: exit status 128")).toBeVisible();
  const release = within(dialog).getByRole("button", { name: "Approve and switch to auto" });
  expect(release).toBeDisabled();
  await user.click(release);
  expect(decisions).toHaveLength(0);
});

const lockfilePatch = [
  "diff --git a/package-lock.json b/package-lock.json",
  "index 1111111..2222222 100644",
  "--- a/package-lock.json",
  "+++ b/package-lock.json",
  "@@ -1,2 +1,2 @@",
  " {",
  '-  "version": "1.0.0"',
  '+  "version": "1.0.1"',
  "",
].join("\n");

function inlineReply(payload: Record<string, unknown>, body = "Done."): ProposalReply {
  return {
    id: 5,
    inReplyTo: 31,
    body,
    dropped: false,
    answers: buildActivity({ kind: "review_comment", actor: "mhernandez", payload }),
  };
}

test("a viewed file folds away and is ticked in the list", async () => {
  const { user } = renderPending();
  const panel = await section();
  await within(panel).findByLabelText("Diff");

  await user.click(within(panel).getByRole("checkbox", { name: "Viewed internal/webhook/deliver.go" }));

  expect(file(panel, "internal/webhook/deliver.go")).toHaveAttribute("data-collapsed", "true");
  expect(within(panel).getByText("2 changed files · 1 viewed")).toBeVisible();
  expect(within(panel).getByRole("button", { name: /^Viewed:\s*deliver.go/ })).toBeVisible();

  await user.click(within(panel).getByRole("checkbox", { name: "Viewed internal/webhook/deliver.go" }));

  expect(file(panel, "internal/webhook/deliver.go")).toHaveAttribute("data-collapsed", "false");
  expect(within(panel).getByText("2 changed files")).toBeVisible();
});

test("the changed files show as a tree of folders", async () => {
  const { user } = renderPending();
  const panel = await section();
  const tree = await within(panel).findByRole("navigation", { name: "Changed files" });

  const folder = within(tree).getByRole("button", { name: "internal/webhook" });
  expect(folder).toHaveAttribute("aria-expanded", "true");
  expect(within(tree).getByRole("button", { name: /^deliver.go, changed/ })).toBeVisible();
  expect(within(tree).getByRole("button", { name: /^deliver_test.go, added/ })).toBeVisible();

  await user.click(folder);

  expect(folder).toHaveAttribute("aria-expanded", "false");
  expect(within(tree).queryByRole("button", { name: /^deliver.go/ })).toBeNull();
});

test("a lockfile waits folded until the author loads it", async () => {
  const { user } = renderPending({
    detail: {
      files: [{ path: "package-lock.json", status: "M", added: 1, deleted: 1 }],
      diff: lockfilePatch,
    },
  });
  const panel = await section();
  await within(panel).findByLabelText("Diff");

  expect(file(panel, "package-lock.json")).toHaveAttribute("data-collapsed", "true");
  expect(within(panel).getByText("large or generated")).toBeVisible();

  await user.click(within(panel).getByRole("button", { name: "Load diff" }));

  expect(file(panel, "package-lock.json")).toHaveAttribute("data-collapsed", "false");
  expect(within(panel).getByLabelText("Diff")).toHaveTextContent('"version": "1.0.1"');
});

test("a comment on a line the change does not show sits at the top of its file", async () => {
  renderPending({
    proposal: {
      replies: [
        inlineReply(
          { path: "internal/webhook/deliver.go", line: 120, side: "RIGHT", body: "Rename this." },
          "Renamed it.",
        ),
      ],
    },
  });
  const panel = await section();
  await within(panel).findByLabelText("Diff");
  const deliver = file(panel, "internal/webhook/deliver.go");

  expect(within(deliver).getByText("On line 120, which this change does not show.")).toBeVisible();
  expect(within(deliver).getByLabelText("Reply to mhernandez")).toHaveValue("Renamed it.");
});

test("a comment on a line of an older commit is not placed on the new code", async () => {
  renderPending({
    proposal: {
      replies: [
        inlineReply({ path: "internal/webhook/deliver.go", line: 34, side: "RIGHT", commit_id: "0ld", body: "Why?" }),
      ],
    },
  });
  const panel = await section();

  expect(await within(panel).findByText("On line 34, which this change does not show.")).toBeVisible();
});

test("a comment on a file outside the diff stays in the list", async () => {
  renderPending({
    proposal: {
      replies: [inlineReply({ path: "internal/webhook/handler.go", line: 8, body: "Handle the error." }, "Fixed it.")],
    },
  });
  const panel = await section();

  expect(await within(panel).findByText("1 reply · posted under your account")).toBeVisible();
  expect(within(within(panel).getByLabelText("Diff")).queryByLabelText("Reply to mhernandez")).toBeNull();
  expect(within(panel).getByLabelText("Reply to mhernandez")).toHaveValue("Fixed it.");
});

test("the split layout and the wrap stay for the next proposal", async () => {
  const { user } = renderPending();
  const panel = await section();
  await within(panel).findByLabelText("Diff");

  await user.click(within(panel).getByRole("radio", { name: "Split" }));
  await user.click(within(panel).getByRole("button", { name: "Wrap lines" }));

  expect(viewer(panel)).toHaveAttribute("data-diff-style", "split");
  expect(viewer(panel)).toHaveAttribute("data-overflow", "wrap");
  expect(JSON.parse(window.localStorage.getItem("diff_preferences") ?? "{}")).toEqual({ style: "split", wrap: true });
});

test("a cut diff says so and names the files it lost", async () => {
  renderPending({
    detail: {
      truncated: true,
      files: [
        { path: "internal/webhook/deliver.go", status: "M", added: 14, deleted: 3 },
        { path: "internal/webhook/deliver_test.go", status: "A", added: 40, deleted: 0 },
        { path: "internal/webhook/retry.go", status: "A", added: 90, deleted: 0 },
      ],
    },
  });
  const panel = await section();

  expect(await within(panel).findByText("The diff is longer than one megabyte and was cut.")).toBeVisible();
  expect(
    within(panel).getByText("The last file shown stops where the cut is, and 1 file after it is not in the diff."),
  ).toBeVisible();
  expect(within(file(panel, "internal/webhook/deliver_test.go")).getByText("cut here")).toBeVisible();
  expect(within(panel).getByText("not in the diff")).toBeVisible();
  expect(within(panel).getByText("3 changed files")).toBeVisible();
});

test("the full window opens the diff with its replies", async () => {
  const { user } = renderPending();
  const panel = await section();
  await within(panel).findByLabelText("Diff");

  await user.click(within(panel).getByRole("button", { name: "Full window" }));

  const dialog = await screen.findByRole("dialog", { name: "Files changed" });
  expect(within(dialog).getByLabelText("Diff")).toHaveTextContent("return retry(ctx, send, req)");
  expect(within(dialog).getByLabelText("Reply to mhernandez")).toBeVisible();
  expect(within(panel).getByText("The diff is open in the full window.")).toBeVisible();
});

test("a rejected push keeps every reply in the list", async () => {
  const { user } = renderPending();
  const panel = await section();
  await within(panel).findByLabelText("Diff");

  await user.click(within(panel).getByRole("button", { name: "Reject push" }));
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("button", { name: /Reject push/ }));

  expect(await within(panel).findByText("2 replies · posted under your account")).toBeVisible();
  expect(within(within(panel).getByLabelText("Diff")).queryByLabelText("Reply to mhernandez")).toBeNull();
});
