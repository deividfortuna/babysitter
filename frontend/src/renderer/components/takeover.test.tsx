import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildActivity, buildWatch } from "@test/fixtures";
import { renderWithProviders } from "@test/test-utils";
import { apiUrl, server, serveApi } from "@test/msw";
import { WatchDetail } from "./watch-detail";
import { WatchRow } from "./watch-row";

function renderWatchDetail() {
  renderWithProviders(<WatchDetail id={42} enabled onNavigate={vi.fn()} onStopped={vi.fn()} onWatchPR={vi.fn()} />);
}

const takenOver = () =>
  buildWatch({
    id: 42,
    takenOverAt: new Date(Date.now() - 4 * 60_000).toISOString(),
    worktreeDir: "/data/worktrees/octo-babysitter-12",
    workBranch: "babysitter/feature/notifications",
    session: { state: "none", pid: 0, logPath: "/data/sessions/42.log" },
    readyBlockers: ["the session is with you"],
  });

function handbackBodies(respond: (body: Record<string, unknown>) => Response) {
  const bodies: Record<string, unknown>[] = [];
  server.use(
    http.post(apiUrl("/api/v1/watches/:id/handback"), async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>;
      bodies.push(body);
      return respond(body);
    }),
  );
  return bodies;
}

test("gives the takeover command of the watch with a copy button", async () => {
  const watch = buildWatch({ id: 42, pendingProposal: 4 });
  serveApi({ watches: [watch], watchById: { 42: watch } });
  const user = userEvent.setup();

  renderWatchDetail();
  await user.click(await screen.findByRole("button", { name: "Continue in terminal" }));
  const dialog = await screen.findByRole("dialog", { name: "Continue in your terminal" });

  expect(within(dialog).getByRole("textbox", { name: "Takeover command" })).toHaveValue(
    "babysitter watch takeover octo/babysitter#12",
  );
  expect(within(dialog).getByText(/proposal 4 is declined/)).toBeInTheDocument();
  await user.click(within(dialog).getByRole("button", { name: "Copy" }));
  expect(await navigator.clipboard.readText()).toBe("babysitter watch takeover octo/babysitter#12");
  expect(within(dialog).getByRole("button", { name: "Copied" })).toBeInTheDocument();
});

test("offers no takeover on a watch that is taken over", async () => {
  const watch = takenOver();
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  expect(await screen.findByRole("button", { name: "Hand back" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Continue in terminal" })).not.toBeInTheDocument();
});

test("shows a watch that is taken over with the worktree, the branch and the push command", async () => {
  const watch = takenOver();
  serveApi({ watches: [watch], watchById: { 42: watch } });
  const user = userEvent.setup();

  renderWatchDetail();

  expect(await screen.findAllByText("with you")).toHaveLength(2);
  expect(screen.getByText("the session is with you")).toBeInTheDocument();
  expect(screen.getByText("/data/worktrees/octo-babysitter-12")).toBeInTheDocument();
  expect(screen.getByText("babysitter/feature/notifications")).toBeInTheDocument();
  expect(screen.getByText("git push origin HEAD:feature/notifications")).toBeInTheDocument();
  expect(screen.getByText(/babysitter watch handback octo\/babysitter#12/)).toBeInTheDocument();
  expect(screen.queryByRole("textbox", { name: "Message to the agent" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "show the last session" })).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Copy push" }));
  expect(await navigator.clipboard.readText()).toBe("git push origin HEAD:feature/notifications");
});

test("hands the session back at once when nothing waits to be confirmed", async () => {
  const watch = takenOver();
  serveApi({ watches: [watch], watchById: { 42: watch } });
  const bodies = handbackBodies(() => HttpResponse.json(buildWatch({ id: 42 })));
  const user = userEvent.setup();

  renderWatchDetail();
  await user.click(await screen.findByRole("button", { name: "Hand back" }));

  await waitFor(() => expect(bodies).toEqual([{}]));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

test("asks before it hands back work that is not pushed", async () => {
  const watch = takenOver();
  serveApi({ watches: [watch], watchById: { 42: watch } });
  const bodies = handbackBodies((body) =>
    body.confirm
      ? HttpResponse.json(buildWatch({ id: 42 }))
      : HttpResponse.json(
          {
            error: { code: "unconfirmed_work", message: "the worktree has work that is not on the pull request" },
            commits: [
              { sha: "5d0b7f1aaaaaaa", subject: "Start the webhook sink in the retry test" },
              { sha: "9a1c3e2bbbbbbb", subject: "Wait for the sink before the first delivery" },
            ],
            files: [" M internal/webhook/deliver_test.go"],
          },
          { status: 409 },
        ),
  );
  const user = userEvent.setup();

  renderWatchDetail();
  await user.click(await screen.findByRole("button", { name: "Hand back" }));
  const dialog = await screen.findByRole("dialog", {
    name: "Hand back octo/babysitter#12 with work that is not pushed?",
  });

  expect(within(dialog).getByText("2 commits the pull request does not have")).toBeInTheDocument();
  expect(within(dialog).getByText(/5d0b7f1 Start the webhook sink in the retry test/)).toBeInTheDocument();
  expect(within(dialog).getByText("1 file changed and not committed")).toBeInTheDocument();
  expect(within(dialog).getByText(/M internal\/webhook\/deliver_test.go/)).toBeInTheDocument();
  await user.click(within(dialog).getByRole("button", { name: "Hand back" }));

  await waitFor(() => expect(bodies).toEqual([{}, { confirm: true }]));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
});

test("says the agent of the author still runs and offers nothing else", async () => {
  const watch = takenOver();
  serveApi({ watches: [watch], watchById: { 42: watch } });
  handbackBodies(() =>
    HttpResponse.json(
      { error: { code: "author_running", message: "the session is still open in your terminal (pid 51920)" } },
      { status: 409 },
    ),
  );
  const user = userEvent.setup();

  renderWatchDetail();
  await user.click(await screen.findByRole("button", { name: "Hand back" }));

  const alert = await screen.findByRole("alert");
  expect(within(alert).getByText("Your agent is still running, pid 51920.")).toBeInTheDocument();
  expect(within(alert).getByText(/Quit it in your terminal, then hand back/)).toBeInTheDocument();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

test("marks the moves of the author and the rows held for the hand-back", async () => {
  const watch = takenOver();
  serveApi({
    watches: [watch],
    watchById: { 42: watch },
    watchActivity: {
      42: [
        buildActivity({
          id: 1,
          kind: "taken_over",
          actor: "",
          summary: "you took over the session in your terminal; proposal 4 declined",
          payload: { by: "author" },
        }),
        buildActivity({ id: 2, kind: "review_comment", actor: "bob", summary: "bob commented on x.go" }),
      ],
    },
  });

  renderWatchDetail();

  expect(await screen.findByText(/taken over · you/)).toBeInTheDocument();
  expect(screen.getByText(/review comment · bob · .* · held for the hand-back/)).toBeInTheDocument();
});

test("shows a watch that is taken over in the list", () => {
  renderWithProviders(<WatchRow watch={takenOver()} onOpen={vi.fn()} />);

  expect(screen.getByText("with you")).toHaveAttribute("title", "With you for 4m");
});
