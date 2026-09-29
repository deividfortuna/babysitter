import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import {
  buildProviders,
  buildPullRequest,
  buildRepo,
  buildRepoConfig,
  buildSettings,
  buildWatch,
} from "@test/fixtures";
import { chooseOption, optionLabels, renderWithProviders } from "@test/test-utils";
import { apiUrl, server, serveApi } from "@test/msw";
import { StartWatchDialog } from "./start-watch-dialog";

const bridge = vi.hoisted(() => ({
  dialog: { pickDirectory: vi.fn<() => Promise<string | null>>(async () => null) },
  theme: { follow: vi.fn() },
}));

vi.mock("@/lib/bridge", () => ({ bridge }));

afterEach(() => {
  vi.clearAllMocks();
});

type User = ReturnType<typeof userEvent.setup>;

function renderDialog(initial: ReturnType<typeof buildPullRequest> | null = null) {
  return renderWithProviders(
    <StartWatchDialog open onOpenChange={vi.fn()} enabled initial={initial} onStarted={vi.fn()} />,
  );
}

function additionalSettings() {
  return screen.getByRole("button", { name: /Additional settings/ });
}

async function openAdditional(user: User) {
  await user.click(additionalSettings());
}

async function waitForCatalog(user: User) {
  await openAdditional(user);
  await waitFor(() => expect(screen.getByLabelText("Agent")).toBeEnabled());
}

async function fillTarget(user: User) {
  await user.type(screen.getByPlaceholderText("search your open PRs, or paste a URL"), "octo/babysitter#12");
  await user.type(screen.getByLabelText("Checkout to copy the worktree from"), "/Users/octo/code/babysitter");
}

test("enables starting when a valid pull request target and checkout are entered", async () => {
  serveApi();
  renderDialog();
  const user = userEvent.setup();

  const start = screen.getByRole("button", { name: "Start watching" });
  expect(start).toBeDisabled();

  await fillTarget(user);

  await waitFor(() => expect(start).toBeEnabled());
});

test("starts without a checkout and leaves the clone to the daemon", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ startBodies });
  renderDialog();
  const user = userEvent.setup();

  await user.type(screen.getByPlaceholderText("search your open PRs, or paste a URL"), "octo/babysitter#12");

  expect(
    screen.getByText(
      "Empty: babysitter clones the repository once into its data folder and makes the private worktree from that clone.",
    ),
  ).toBeVisible();
  expect(screen.getByText("babysitter watch start octo/babysitter#12 --no-checkout")).toBeVisible();
  const start = screen.getByRole("button", { name: "Start watching" });
  await waitFor(() => expect(start).toBeEnabled());
  await user.click(start);

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).not.toHaveProperty("sourceDir");
});

test("opens again with an empty search after the author typed a target", async () => {
  serveApi();
  const user = userEvent.setup();
  const { rerender } = renderWithProviders(
    <StartWatchDialog open onOpenChange={vi.fn()} enabled initial={null} onStarted={vi.fn()} />,
  );
  await fillTarget(user);

  rerender(<StartWatchDialog open={false} onOpenChange={vi.fn()} enabled initial={null} onStarted={vi.fn()} />);
  rerender(<StartWatchDialog open onOpenChange={vi.fn()} enabled initial={null} onStarted={vi.fn()} />);

  expect(await screen.findByPlaceholderText("search your open PRs, or paste a URL")).toHaveValue("");
});

test("does not allow selecting a pull request that is already watched", async () => {
  serveApi({ pullRequests: [buildPullRequest({ title: "Already watched" })], watches: [buildWatch()] });
  renderDialog();
  const user = userEvent.setup();

  const pullRequest = await screen.findByRole("option", { name: /#12 Already watched/ });
  expect(pullRequest).toHaveAttribute("aria-disabled", "true");

  await user.click(pullRequest);
  expect(screen.getByPlaceholderText("search your open PRs, or paste a URL")).toBeVisible();
});

test("fills the checkout from the Electron directory picker", async () => {
  serveApi();
  bridge.dialog.pickDirectory.mockResolvedValueOnce("/Users/octo/code/babysitter");
  renderDialog();
  const user = userEvent.setup();

  await user.click(screen.getByRole("button", { name: "Choose…" }));

  expect(await screen.findByDisplayValue("/Users/octo/code/babysitter")).toBeVisible();
});

test("says the checkout was remembered for the repository of the pull request", async () => {
  window.localStorage.setItem("checkout_dir:octo/babysitter", "/Users/octo/code/babysitter");
  serveApi();
  renderDialog(buildPullRequest());
  const user = userEvent.setup();

  expect(screen.getByText(/^Remembered for octo\/babysitter\./)).toBeVisible();

  await user.type(screen.getByLabelText("Checkout to copy the worktree from"), "-copy");

  expect(screen.queryByText(/Remembered for/)).toBeNull();
  expect(screen.getByText("A private worktree is made next to it; your checkout is never touched.")).toBeVisible();
});

test("drops the remembered hint when the pull request changes to a repository with no remembered checkout", async () => {
  window.localStorage.setItem("checkout_dir:octo/babysitter", "/Users/octo/code/babysitter");
  serveApi({ pullRequests: [buildPullRequest({ repo: "octo/other", number: 7, title: "Other repository" })] });
  renderDialog(buildPullRequest());
  const user = userEvent.setup();

  await user.click(screen.getByRole("button", { name: "Change" }));
  await user.click(await screen.findByRole("option", { name: /#7 Other repository/ }));

  expect(screen.queryByText(/Remembered for/)).toBeNull();
});

test("drops the remembered hint when the picked pull request is cleared", async () => {
  window.localStorage.setItem("checkout_dir:octo/babysitter", "/Users/octo/code/babysitter");
  serveApi();
  renderDialog(buildPullRequest());
  const user = userEvent.setup();

  await user.click(screen.getByRole("button", { name: "Change" }));

  expect(screen.queryByText(/Remembered for/)).toBeNull();
});

test("closes and notifies the caller after a watch starts successfully", async () => {
  const onOpenChange = vi.fn();
  const onStarted = vi.fn();
  const startedWatch = buildWatch({ id: 42, number: 12, repo: "octo/babysitter" });
  serveApi({ startedWatch });
  const user = userEvent.setup();

  renderWithProviders(
    <StartWatchDialog open onOpenChange={onOpenChange} enabled initial={null} onStarted={onStarted} />,
  );

  await fillTarget(user);
  await waitForCatalog(user);
  await chooseOption(user, screen.getByLabelText("Agent"), /Copilot/);
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => {
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onStarted).toHaveBeenCalledWith(startedWatch);
  });
});

test("folds the additional settings, and names what the watch will use", async () => {
  serveApi({ settings: buildSettings({ approvalMode: "auto", approvalsRequired: 2, mergeMethod: "squash" }) });
  renderDialog();
  const user = userEvent.setup();

  expect(additionalSettings()).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByLabelText("Approval mode")).toBeNull();
  await waitFor(() => expect(additionalSettings()).toHaveTextContent("Claude · auto · 2 approvals · squash"));

  await openAdditional(user);

  expect(additionalSettings()).toHaveAttribute("aria-expanded", "true");
  expect(additionalSettings()).toHaveTextContent("Default takes the repository, then the daemon");
  for (const label of [
    "Agent",
    "Model",
    "Effort",
    "Approval mode",
    "Approve a clean rebase on its own",
    "Approvals before ready to merge",
    "Merge method",
    "Report existing review items",
    "Include my own comments",
    "Keep the worktree when the watch stops",
  ]) {
    expect(screen.getByLabelText(label)).toBeVisible();
  }
});

const INHERITED_FIELDS = [
  "provider",
  "model",
  "effort",
  "approvalsRequired",
  "mergeMethod",
  "includeExisting",
  "includeOwn",
  "keepWorktree",
  "approvalMode",
  "autoApproveRebase",
  "mergeWhenReady",
  "branchUpdate",
  "updateOnGitHub",
];

test("starts with the defaults while the additional settings stay folded", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ settings: buildSettings({ approvalsRequired: 2, includeOwn: true }), startBodies });
  renderDialog();
  const user = userEvent.setup();

  await fillTarget(user);
  await waitFor(() => expect(screen.getByRole("button", { name: "Start watching" })).toBeEnabled());
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toMatchObject({ target: "octo/babysitter#12" });
  for (const field of INHERITED_FIELDS) {
    expect(startBodies[0]).not.toHaveProperty(field);
  }
});

test("each field opens on the value of the daemon", async () => {
  serveApi({
    settings: buildSettings({
      provider: "copilot",
      model: "gpt-5.3-codex",
      approvalMode: "manual",
      autoApproveRebase: true,
      approvalsRequired: 2,
      mergeMethod: "rebase",
      includeExisting: true,
      includeOwn: true,
      keepWorktree: true,
    }),
  });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  await waitFor(() => expect(screen.getByLabelText("Agent")).toHaveTextContent("Default (Copilot)"));
  expect(screen.getByLabelText("Model")).toHaveTextContent("Default (GPT-5.3 Codex)");
  expect(screen.getByLabelText("Approval mode")).toHaveTextContent("Default (manual)");
  expect(screen.getByLabelText("Merge method")).toHaveTextContent("Default (rebase)");
  expect(screen.getByLabelText("Approvals before ready to merge")).toHaveValue(2);
  expect(screen.getByRole("switch", { name: "Approve a clean rebase on its own" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Report existing review items" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Include my own comments" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Keep the worktree when the watch stops" })).toBeChecked();
});

test("the override of the repository beats the daemon", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({
    settings: buildSettings({
      approvalMode: "manual",
      approvalsRequired: 2,
      mergeMethod: "rebase",
      keepWorktree: true,
    }),
    repos: [buildRepo()],
    repoConfig: buildRepoConfig({
      overrides: {
        provider: "copilot",
        model: "",
        effort: "",
        approvalMode: "auto",
        mergeMethod: "squash",
        branchUpdate: "",
        approvalsRequired: null,
        keepWorktree: false,
      },
    }),
    startBodies,
  });
  renderDialog();
  const user = userEvent.setup();
  await fillTarget(user);
  await waitFor(() =>
    expect(additionalSettings()).toHaveTextContent("Copilot · auto · rule of the base branch · squash"),
  );
  await openAdditional(user);

  expect(screen.getByLabelText("Agent")).toHaveTextContent("Default (Copilot)");
  expect(screen.getByLabelText("Model")).toHaveTextContent("Default (Provider default)");
  expect(screen.getByLabelText("Approval mode")).toHaveTextContent("Default (auto)");
  expect(screen.getByLabelText("Approvals before ready to merge")).toHaveValue(null);
  expect(screen.getByLabelText("Merge method")).toHaveTextContent("Default (squash)");
  expect(screen.getByRole("switch", { name: "Keep the worktree when the watch stops" })).not.toBeChecked();

  await user.click(screen.getByRole("button", { name: "Start watching" }));
  await waitFor(() => expect(startBodies).toHaveLength(1));
  for (const field of INHERITED_FIELDS) {
    expect(startBodies[0]).not.toHaveProperty(field);
  }
});

test("a watch can start with its own branch update", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ settings: buildSettings({ branchUpdate: "rebase", updateOnGitHub: true }), startBodies });
  renderDialog();
  const user = userEvent.setup();
  await fillTarget(user);
  await openAdditional(user);

  expect(screen.getByLabelText("Branch behind its base")).toHaveTextContent("Default (rebase)");
  await chooseOption(user, screen.getByLabelText("Branch behind its base"), "Merge");
  await user.click(screen.getByRole("switch", { name: "Update the branch on GitHub first" }));
  expect(screen.getByText(/--branch-update merge --update-on-github=false/)).toBeVisible();

  await user.click(screen.getByRole("button", { name: "Start watching" }));
  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toMatchObject({ branchUpdate: "merge", updateOnGitHub: false });
});

test("a Dependabot pull request leaves the branch update to the bot", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ startBodies });
  renderDialog(buildPullRequest({ author: "dependabot[bot]" }));
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("Checkout to copy the worktree from"), "/Users/octo/code/babysitter");
  await openAdditional(user);

  expect(screen.getByText("Dependabot owns the branch, so only the bot updates it.")).toBeVisible();
  expect(screen.getByLabelText("Branch behind its base")).toBeDisabled();
  expect(screen.getByRole("switch", { name: "Update the branch on GitHub first" })).toBeDisabled();
  expect(screen.queryByText(/--branch-update|--update-on-github/)).toBeNull();

  await user.click(screen.getByRole("button", { name: "Start watching" }));
  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).not.toHaveProperty("branchUpdate");
  expect(startBodies[0]).not.toHaveProperty("updateOnGitHub");
});

test("a Dependabot pull request drops the branch update chosen before it was picked", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({
    startBodies,
    pullRequests: [buildPullRequest({ number: 7, title: "Bump lodash", author: "dependabot[bot]" })],
  });
  renderDialog(buildPullRequest());
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("Checkout to copy the worktree from"), "/Users/octo/code/babysitter");
  await openAdditional(user);
  await chooseOption(user, screen.getByLabelText("Branch behind its base"), "Merge");
  await user.click(screen.getByRole("switch", { name: "Update the branch on GitHub first" }));

  await user.click(screen.getByRole("button", { name: "Change" }));
  await user.click(await screen.findByRole("option", { name: /#7 Bump lodash/ }));

  expect(screen.queryByText(/--branch-update|--update-on-github/)).toBeNull();
  await user.click(screen.getByRole("button", { name: "Start watching" }));
  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).not.toHaveProperty("branchUpdate");
  expect(startBodies[0]).not.toHaveProperty("updateOnGitHub");
});

test("holds the start until the list of repositories has landed", async () => {
  serveApi({ repos: [buildRepo()] });
  let answer: (() => void) | undefined;
  const landed = new Promise<void>((resolve) => {
    answer = resolve;
  });
  server.use(
    http.get(apiUrl("/api/v1/repos"), async () => {
      await landed;
      return HttpResponse.json({ repos: [buildRepo()] });
    }),
  );
  renderDialog();
  const user = userEvent.setup();

  await fillTarget(user);
  await openAdditional(user);
  await waitFor(() => expect(screen.getByLabelText("Approval mode")).toHaveTextContent("Default (manual)"));
  expect(screen.getByRole("button", { name: "Start watching" })).toBeDisabled();

  answer?.();
  await waitFor(() => expect(screen.getByRole("button", { name: "Start watching" })).toBeEnabled());
});

test("holds the start and says so when the settings of the repository cannot be read", async () => {
  serveApi({ repos: [buildRepo()] });
  server.use(
    http.get(apiUrl("/api/v1/repos/:id/config"), () =>
      HttpResponse.json({ error: { code: "store_failed", message: "disk full" } }, { status: 500 }),
    ),
  );
  renderDialog();
  const user = userEvent.setup();

  await fillTarget(user);

  expect(await screen.findByText(/settings of the repository could not be read/)).toBeVisible();
  expect(screen.getByRole("button", { name: "Start watching" })).toBeDisabled();
});

test("holds the start until the settings of the repository have landed", async () => {
  serveApi({ repos: [buildRepo()] });
  let answer: (() => void) | undefined;
  const landed = new Promise<void>((resolve) => {
    answer = resolve;
  });
  server.use(
    http.get(apiUrl("/api/v1/repos/:id/config"), async () => {
      await landed;
      return HttpResponse.json(buildRepoConfig());
    }),
  );
  renderDialog();
  const user = userEvent.setup();

  await fillTarget(user);
  expect(screen.getByRole("button", { name: "Start watching" })).toBeDisabled();

  answer?.();
  await waitFor(() => expect(screen.getByRole("button", { name: "Start watching" })).toBeEnabled());
});

test("the summary follows the choices of the author", async () => {
  serveApi();
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  await chooseOption(user, screen.getByLabelText("Agent"), /Copilot/);
  await chooseOption(user, screen.getByLabelText("Model"), "GPT-5.3 Codex");
  await user.type(screen.getByLabelText("Approvals before ready to merge"), "1");
  await openAdditional(user);

  expect(additionalSettings()).toHaveTextContent("Copilot GPT-5.3 Codex · manual · 1 approval · repository default");
});

test("shows the logo of each agent", async () => {
  serveApi();
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  expect(screen.getByLabelText("Agent").querySelector("img")).not.toBeNull();
  await user.click(screen.getByLabelText("Agent"));
  for (const name of ["Claude", "Copilot"]) {
    expect((await screen.findByRole("option", { name })).querySelector("img")).not.toBeNull();
  }
});

test("offers the models of the selected provider and sends the one picked", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ startBodies });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  const models = screen.getByLabelText("Model");
  expect(await optionLabels(user, models)).toEqual(["Default (Provider default)", "Opus", "Sonnet", "Haiku"]);

  await chooseOption(user, screen.getByLabelText("Agent"), /Copilot/);
  expect(await optionLabels(user, models)).toEqual(["Provider default", "GPT-5.3 Codex"]);

  await chooseOption(user, models, "GPT-5.3 Codex");
  await fillTarget(user);
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toMatchObject({ provider: "copilot", model: "gpt-5.3-codex" });
});

test("a model without an agent runs on the agent of the defaults", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ startBodies });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  await chooseOption(user, screen.getByLabelText("Model"), "Sonnet");
  await fillTarget(user);
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toMatchObject({ model: "sonnet" });
  expect(startBodies[0]).not.toHaveProperty("provider");
});

test("offers the efforts of the model and sends the one picked", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ startBodies, settings: buildSettings({ model: "opus", effort: "high" }) });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  const effort = screen.getByLabelText("Effort");
  await waitFor(() => expect(effort).toHaveTextContent("Default (High)"));
  await chooseOption(user, effort, "Extra high");
  await fillTarget(user);
  expect(screen.getByText("babysitter watch start octo/babysitter#12 --effort xhigh")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toMatchObject({ effort: "xhigh" });
  expect(startBodies[0]).not.toHaveProperty("model");
});

test("a new model starts from its own default effort", async () => {
  serveApi({ settings: buildSettings({ model: "opus", effort: "high" }) });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  const effort = screen.getByLabelText("Effort");
  await chooseOption(user, effort, "Max");
  await chooseOption(user, screen.getByLabelText("Model"), "Sonnet");
  expect(effort).toHaveTextContent("Model default");

  await chooseOption(user, screen.getByLabelText("Model"), "Haiku");
  expect(effort).toBeDisabled();
});

test("resets the model when the provider changes", async () => {
  serveApi();
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  const models = screen.getByLabelText("Model");
  await chooseOption(user, models, "Sonnet");
  expect(models).toHaveTextContent("Sonnet");

  await chooseOption(user, screen.getByLabelText("Agent"), /Copilot/);
  expect(models).toHaveTextContent("Provider default");
});

test("does not offer a provider whose command the daemon did not find", async () => {
  serveApi({ providers: buildProviders([{}, { available: false }]) });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  await user.click(screen.getByLabelText("Agent"));
  const copilot = await screen.findByRole("option", { name: /Copilot/ });
  expect(copilot).toHaveAttribute("aria-disabled", "true");
  expect(copilot).toHaveTextContent("command not found");
});

test("starts on the first installed provider when the default one is missing", async () => {
  serveApi({ providers: buildProviders([{ available: false }, {}]) });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  expect(screen.getByLabelText("Agent")).toHaveTextContent("Copilot");
  expect(await optionLabels(user, screen.getByLabelText("Model"))).toEqual(["Provider default", "GPT-5.3 Codex"]);
});

test("sends the approvals and the merge method of the watch", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ startBodies });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  const approvals = screen.getByLabelText("Approvals before ready to merge");
  expect(approvals).toHaveValue(null);
  await user.type(approvals, "2");
  await chooseOption(user, screen.getByLabelText("Merge method"), "Rebase");
  await fillTarget(user);
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toMatchObject({ approvalsRequired: 2, mergeMethod: "rebase" });
});

test("the repository default of GitHub is a choice apart from Default", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ settings: buildSettings({ mergeMethod: "squash" }), startBodies });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  await chooseOption(user, screen.getByLabelText("Merge method"), "Repository default");
  await fillTarget(user);
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toHaveProperty("mergeMethod", "");
});

test("refuses approvals that are not a whole number", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ startBodies });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  await fillTarget(user);
  await user.type(screen.getByLabelText("Approvals before ready to merge"), "2.5");

  const start = screen.getByRole("button", { name: "Start watching" });
  await waitFor(() => expect(start).toBeDisabled());
  expect(screen.getByText("Use a whole number from 0, or leave it empty.")).toBeInTheDocument();
  expect(startBodies).toHaveLength(0);

  await user.clear(screen.getByLabelText("Approvals before ready to merge"));
  await user.type(screen.getByLabelText("Approvals before ready to merge"), "3");
  await waitFor(() => expect(start).toBeEnabled());
  await user.click(start);
  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toMatchObject({ approvalsRequired: 3 });
});

test("leaves the approvals out of the body unless the author touches the field", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ startBodies });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  await fillTarget(user);
  await user.click(screen.getByRole("button", { name: "Start watching" }));
  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).not.toHaveProperty("approvalsRequired");

  await user.type(screen.getByLabelText("Approvals before ready to merge"), "0");
  await user.click(screen.getByRole("button", { name: "Start watching" }));
  await waitFor(() => expect(startBodies).toHaveLength(2));
  expect(startBodies[1]).toMatchObject({ approvalsRequired: 0 });
});

test("clearing the approvals asks for the base branch over a default that names a number", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ settings: buildSettings({ approvalsRequired: 2 }), startBodies });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  await waitFor(() => expect(screen.getByLabelText("Approvals before ready to merge")).toHaveValue(2));
  await user.clear(screen.getByLabelText("Approvals before ready to merge"));
  await fillTarget(user);
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toHaveProperty("approvalsRequired", null);
});

test("the fields scroll, and the title and the buttons stay out of that scroll", async () => {
  serveApi();
  renderDialog();
  const user = userEvent.setup();
  await openAdditional(user);

  const scroller = screen.getByLabelText("Checkout to copy the worktree from").closest(".overflow-y-auto");
  expect(scroller).not.toBeNull();

  expect(scroller!.contains(screen.getByLabelText("Include my own comments"))).toBe(true);
  expect(scroller!.contains(screen.getByRole("button", { name: "Start watching" }))).toBe(false);
  expect(scroller!.contains(screen.getByRole("heading", { name: "Watch a pull request" }))).toBe(false);
});

test("sends only the options the author touched", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ settings: buildSettings({ includeOwn: true, mergeMethod: "rebase", approvalsRequired: 2 }), startBodies });
  renderDialog();
  const user = userEvent.setup();

  await fillTarget(user);
  await waitFor(() => expect(screen.getByRole("button", { name: "Start watching" })).toBeEnabled());
  await openAdditional(user);
  await user.click(screen.getByRole("switch", { name: "Include my own comments" }));
  await user.click(screen.getByRole("switch", { name: "Keep the worktree when the watch stops" }));
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toHaveProperty("includeOwn", false);
  expect(startBodies[0]).toHaveProperty("keepWorktree", true);
  expect(startBodies[0]).not.toHaveProperty("includeExisting");
  expect(startBodies[0]).not.toHaveProperty("approvalsRequired");
  expect(startBodies[0]).not.toHaveProperty("mergeMethod");
});

test("holds the start until the settings it shows have landed", async () => {
  serveApi();
  let answer: (() => void) | undefined;
  const landed = new Promise<void>((resolve) => {
    answer = resolve;
  });
  server.use(
    http.get(apiUrl("/api/v1/settings"), async () => {
      await landed;
      return HttpResponse.json(buildSettings());
    }),
  );
  renderDialog();
  const user = userEvent.setup();

  await fillTarget(user);
  expect(screen.getByRole("button", { name: "Start watching" })).toBeDisabled();

  answer?.();
  await waitFor(() => expect(screen.getByRole("button", { name: "Start watching" })).toBeEnabled());
});

test("holds the start button when the settings of the daemon cannot be read", async () => {
  serveApi({ settingsFail: true });
  renderDialog();
  const user = userEvent.setup();

  await fillTarget(user);

  expect(await screen.findByText(/settings of the daemon could not be read/)).toBeVisible();
  expect(screen.getByRole("button", { name: "Start watching" })).toBeDisabled();
});

test("the approval mode and the clean rebase go with the start when the author sets them", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ settings: buildSettings({ approvalMode: "manual", autoApproveRebase: false }), startBodies });
  renderDialog();
  const user = userEvent.setup();
  await openAdditional(user);

  await waitFor(() => expect(screen.getByLabelText("Approval mode")).toHaveTextContent("Default (manual)"));
  const rebase = screen.getByRole("switch", { name: "Approve a clean rebase on its own" });
  expect(rebase).not.toBeChecked();
  await user.click(rebase);
  await fillTarget(user);
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toHaveProperty("autoApproveRebase", true);
  expect(startBodies[0]).not.toHaveProperty("approvalMode");
});

test("a new watch in auto sends its mode, and the clean rebase means nothing there", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ settings: buildSettings({ approvalMode: "manual" }), startBodies });
  renderDialog();
  const user = userEvent.setup();
  await openAdditional(user);

  await waitFor(() => expect(screen.getByLabelText("Approval mode")).toHaveTextContent("Default (manual)"));
  await user.click(screen.getByRole("switch", { name: "Approve a clean rebase on its own" }));
  await chooseOption(user, screen.getByLabelText("Approval mode"), "auto");
  expect(screen.getByRole("switch", { name: "Approve a clean rebase on its own" })).toBeDisabled();
  await fillTarget(user);
  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toHaveProperty("approvalMode", "auto");
  expect(startBodies[0]).not.toHaveProperty("autoApproveRebase");
});

test("the approval mode says what each mode does", async () => {
  serveApi({ settings: buildSettings({ approvalMode: "manual" }) });
  renderDialog();
  const user = userEvent.setup();
  await openAdditional(user);

  await waitFor(() => expect(screen.getByLabelText("Approval mode")).toHaveTextContent("Default (manual)"));
  expect(screen.getByText("Manual holds each turn until you approve it. Nothing goes out before.")).toBeVisible();

  await chooseOption(user, screen.getByLabelText("Approval mode"), "auto");
  expect(screen.getByText("Auto pushes and posts as soon as each turn of the agent ends.")).toBeVisible();
  expect(screen.queryByText(/until you approve/)).toBeNull();
});

test("the command beside the start button carries the choices of the author", async () => {
  serveApi({ settings: buildSettings({ approvalMode: "manual" }) });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);
  await waitFor(() => expect(screen.getByLabelText("Approval mode")).toHaveTextContent("Default (manual)"));

  await user.type(screen.getByPlaceholderText("search your open PRs, or paste a URL"), "octo/babysitter#12");
  await user.type(screen.getByLabelText("Checkout to copy the worktree from"), "/Users/octo/code/babysitter");
  expect(screen.getByText("babysitter watch start octo/babysitter#12")).toBeVisible();

  await chooseOption(user, screen.getByLabelText("Model"), "Sonnet");
  await user.click(screen.getByRole("switch", { name: "Approve a clean rebase on its own" }));
  expect(
    screen.getByText("babysitter watch start octo/babysitter#12 --model sonnet --auto-approve-rebase"),
  ).toBeVisible();

  await chooseOption(user, screen.getByLabelText("Approval mode"), "auto");
  await chooseOption(user, screen.getByLabelText("Agent"), /Copilot/);
  expect(
    screen.getByText("babysitter watch start octo/babysitter#12 --provider copilot --approval-mode auto"),
  ).toBeVisible();
});

test("merge when ready is off by default, and a switch on sends it and adds the flag to the command", async () => {
  const startBodies: Record<string, unknown>[] = [];
  serveApi({ startBodies });
  renderDialog();
  const user = userEvent.setup();
  await waitForCatalog(user);

  const mergeWhenReady = screen.getByRole("switch", { name: "Merge when ready" });
  expect(mergeWhenReady).not.toBeChecked();
  expect(screen.getByText("Merge with the method above as soon as the watch is ready to merge.")).toBeVisible();

  await fillTarget(user);
  await user.click(mergeWhenReady);
  expect(screen.getByText("babysitter watch start octo/babysitter#12 --merge-when-ready")).toBeVisible();

  await user.click(screen.getByRole("button", { name: "Start watching" }));

  await waitFor(() => expect(startBodies).toHaveLength(1));
  expect(startBodies[0]).toMatchObject({ mergeWhenReady: true });
});
