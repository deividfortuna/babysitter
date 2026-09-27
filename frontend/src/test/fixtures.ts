import type { Activity } from "@/hooks/useWatchActivity";
import type { Notification } from "@/hooks/useNotifications";
import type { Proposal, ProposalDetail } from "@/hooks/useProposals";
import type { PullRequest } from "@/hooks/usePulls";
import type { Provider } from "@/hooks/useProviders";
import type { RateLimit } from "@/hooks/useRateLimit";
import type { QueuedPullRequest, Repo, RepoConfig } from "@/hooks/useRepos";
import type { Settings } from "@/hooks/useSettings";
import type { Viewer } from "@/hooks/useViewer";
import type { Watch } from "@/hooks/useWatches";

export function buildSettings(overrides: Partial<Settings> = {}): Settings {
  return {
    pollIntervalSeconds: 60,
    watchIntervalSeconds: 180,
    approvalsRequired: null,
    mergeMethod: "",
    includeExisting: false,
    includeOwn: false,
    keepWorktree: false,
    notificationsEnabled: true,
    notificationSound: true,
    mutedNotificationKinds: [],
    approvalMode: "manual",
    autoApproveRebase: false,
    ...overrides,
  };
}

export function buildNotification(overrides: Partial<Notification> = {}): Notification {
  return {
    id: 1,
    watchId: 42,
    kind: "review",
    repo: "octocat/hello-world",
    number: 7,
    title: "PR #7",
    body: "bob commented: please rename this",
    url: "https://github.com/octocat/hello-world/pull/7",
    createdAt: "2026-09-21T12:00:00Z",
    ...overrides,
  };
}

type WatchSummary = NonNullable<Watch["summary"]>;
type StoppedWatchOverrides = Omit<Partial<Watch>, "summary"> & { summary?: Partial<WatchSummary> };

export function buildWatch(overrides: Partial<Watch> = {}): Watch {
  return {
    agentSession: "0d5f4b5e-6f1e-4f8e-9c9a-2d4b7e1c3a10",
    approvalMode: "auto",
    autoApproveRebase: false,
    approvalsRequired: 1,
    author: "octocat",
    baseRef: "main",
    checkStates: {},
    dependabot: false,
    greenSha: "1234567890",
    headRef: "feature/notifications",
    headSha: "1234567890",
    provider: "claude",
    model: "",
    id: 42,
    includeExisting: false,
    includeOwn: false,
    lastError: "",
    lastPollAt: null,
    mergeableState: "clean",
    mergeMethod: "",
    mergeWhenReady: false,
    number: 12,
    prState: "open",
    readyBlockers: [],
    readySince: null,
    repo: "octo/babysitter",
    session: { state: "idle", pid: 4242, startedAt: "2026-01-01T00:00:00Z", logPath: "/data/sessions/42.log" },
    sourceDir: "",
    startedAt: "2026-01-01T00:00:00Z",
    status: "active",
    stopReason: "",
    stoppedAt: null,
    title: "Add notifications",
    url: "https://github.com/octo/babysitter/pull/12",
    workBranch: "",
    worktreeDir: "",
    ...overrides,
  };
}

export function buildStoppedWatch(overrides: StoppedWatchOverrides = {}): Watch {
  const { summary: summaryOverride, ...watchOverrides } = overrides;
  const summary: WatchSummary = {
    activity: { comment: 2, commit: 1 },
    checks: "green",
    detail: "Everything is ready.",
    headSha: "1234567890",
    mergeableState: "clean",
    messages: 3,
    prState: "open",
    reason: "user",
    worktreeRemoved: true,
  };
  return buildWatch({
    ...watchOverrides,
    session: { state: "none", pid: 0, logPath: "" },
    status: "stopped",
    stopReason: "user",
    stoppedAt: "2026-01-01T01:00:00Z",
    summary: { ...summary, ...summaryOverride },
  });
}

export function buildRepo(overrides: Partial<Repo> = {}): Repo {
  return {
    addedAt: "2026-09-16T00:00:00Z",
    fullName: "octo/babysitter",
    id: 1,
    lastError: "",
    lastSyncedAt: "2026-09-16T09:00:00Z",
    name: "babysitter",
    owner: "octo",
    ...overrides,
  };
}

export function buildRepoConfig(overrides: Partial<RepoConfig> = {}): RepoConfig {
  return {
    repoId: 1,
    repo: "octo/babysitter",
    checkoutDir: "",
    autoStartMine: false,
    autoStartMineSince: null,
    includeDrafts: false,
    autoWatchDependabot: false,
    autoWatchDependabotSince: null,
    overrides: { provider: "", model: "", approvalMode: "", mergeMethod: "" },
    dependabotScope: "patch",
    dependabotApproval: "never",
    dependabotLimit: 1,
    ...overrides,
  };
}

export function buildQueuedPullRequest(overrides: Partial<QueuedPullRequest> = {}): QueuedPullRequest {
  return {
    number: 30,
    title: "Bump golang.org/x/net from 0.33.0 to 0.34.0",
    url: "https://github.com/octo/babysitter/pull/30",
    updateType: "minor",
    createdAt: "2026-09-16T08:00:00Z",
    position: 1,
    ...overrides,
  };
}

export function buildPullRequest(overrides: Partial<PullRequest> = {}): PullRequest {
  return {
    additions: 1,
    approvals: 1,
    assignees: [],
    author: "octocat",
    baseRef: "main",
    changesRequested: 0,
    ciStatus: "success",
    createdAt: "2026-09-16T08:00:00Z",
    deletions: 0,
    draft: false,
    fork: false,
    headRef: "feature/notifications",
    headSha: "1234567890",
    htmlUrl: "https://github.com/octo/babysitter/pull/12",
    labels: [],
    mergeableState: "clean",
    number: 12,
    repo: "octo/babysitter",
    requestedReviewers: [],
    reviewDecision: "approved",
    state: "open",
    syncedAt: "2026-09-16T09:00:00Z",
    title: "Add notifications",
    updatedAt: "2026-09-16T09:00:00Z",
    ...overrides,
  };
}

export function buildProposal(overrides: Partial<Proposal> = {}): Proposal {
  return {
    number: 3,
    status: "pending",
    headSha: "9f3c2a1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    baseSha: "9f3c2a1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    workSha: "4e7d0b8bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    hasPush: true,
    pushRejected: false,
    openedAt: "2026-01-01T00:00:00Z",
    endedAt: "2026-01-01T00:05:00Z",
    replies: [
      {
        id: 5,
        inReplyTo: 31,
        body: "Moved the retry into deliver and added a test.",
        dropped: false,
        answers: buildActivity({
          id: 7,
          kind: "review_comment",
          actor: "mhernandez",
          summary: "mhernandez commented on internal/webhook/deliver.go:34",
          payload: {
            path: "internal/webhook/deliver.go",
            line: 34,
            body: "This retry belongs in deliver, not in the handler.",
          },
        }),
      },
      {
        id: 6,
        body: "The failure is on main too.",
        dropped: false,
      },
    ],
    ...overrides,
  };
}

export function buildProposalDetail(overrides: Partial<ProposalDetail> = {}): ProposalDetail {
  return {
    ...buildProposal(),
    commits: [
      { sha: "3b1e9c4cccccccccccccccccccccccccccccccc", subject: "Move the retry into deliver" },
      { sha: "7a20d55ddddddddddddddddddddddddddddddddd", subject: "Test the retry of a delivery" },
    ],
    files: [
      { path: "internal/webhook/deliver.go", status: "M", added: 14, deleted: 3 },
      { path: "internal/webhook/deliver_test.go", status: "A", added: 40, deleted: 0 },
    ],
    diff: [
      "diff --git a/internal/webhook/deliver.go b/internal/webhook/deliver.go",
      "index 1a2b3c4..5d6e7f8 100644",
      "--- a/internal/webhook/deliver.go",
      "+++ b/internal/webhook/deliver.go",
      "@@ -31,6 +31,6 @@ func Deliver(ctx context.Context, req Request) error {",
      " \tif err := validate(req); err != nil {",
      " \t\treturn err",
      " \t}",
      "-\treturn send(ctx, req)",
      "+\treturn retry(ctx, send, req)",
      " }",
      " ",
      "diff --git a/internal/webhook/deliver_test.go b/internal/webhook/deliver_test.go",
      "new file mode 100644",
      "index 0000000..9a8b7c6",
      "--- /dev/null",
      "+++ b/internal/webhook/deliver_test.go",
      "@@ -0,0 +1 @@",
      "+func TestDeliverRetries(t *testing.T) {}",
      "",
    ].join("\n"),
    truncated: false,
    ...overrides,
  };
}

export function buildActivity(overrides: Partial<Activity> = {}): Activity {
  return {
    actor: "octocat",
    at: "2026-09-16T00:00:00Z",
    id: 1,
    kind: "comment",
    payload: { path: "writer.go", line: 118 },
    ref: "refs/heads/main",
    reported: true,
    summary: "Looks good",
    url: "https://github.com/octo/babysitter/pull/12#discussion_r1",
    watchId: 42,
    ...overrides,
  };
}

export function buildProviders(overrides: Partial<Provider>[] = []): Provider[] {
  const base: Provider[] = [
    {
      id: "claude",
      label: "Claude",
      available: true,
      models: [
        { id: "", label: "Provider default" },
        { id: "opus", label: "Opus" },
        { id: "sonnet", label: "Sonnet" },
      ],
    },
    {
      id: "copilot",
      label: "Copilot",
      available: true,
      models: [
        { id: "", label: "Provider default" },
        { id: "gpt-5.3-codex", label: "GPT-5.3 Codex" },
      ],
    },
  ];
  return base.map((provider, i) => ({ ...provider, ...overrides[i] }));
}

export function buildRateLimit(overrides: Partial<RateLimit> = {}): RateLimit {
  return {
    state: "ok",
    limit: 5000,
    remaining: 4212,
    resetAt: "2026-09-24T12:38:00Z",
    ...overrides,
  };
}

export function buildViewer(overrides: Partial<Viewer> = {}): Viewer {
  return {
    login: "deividfortuna",
    name: "Deivid Fortuna",
    avatarUrl: "https://avatars.githubusercontent.com/u/1?v=4",
    ...overrides,
  };
}
