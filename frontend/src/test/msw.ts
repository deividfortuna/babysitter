import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { setApiBaseUrl } from "@/lib/api-client";
import {
  buildActivity,
  buildProviders,
  buildRepo,
  buildRepoConfig,
  buildSettings,
  buildViewer,
  buildWatch,
} from "./fixtures";
import type { Activity } from "@/hooks/useWatchActivity";
import type { Notification } from "@/hooks/useNotifications";
import type { Proposal, ProposalDetail } from "@/hooks/useProposals";
import type { PullRequest } from "@/hooks/usePulls";
import type { Provider } from "@/hooks/useProviders";
import type { RateLimit } from "@/hooks/useRateLimit";
import type { QueuedPullRequest, Repo, RepoConfig, RepoConfigUpdate } from "@/hooks/useRepos";
import type { Settings } from "@/hooks/useSettings";
import type { Viewer } from "@/hooks/useViewer";
import type { Watch } from "@/hooks/useWatches";
import type { LogLevel } from "../shared/logs";

export const testApiBaseUrl = "http://127.0.0.1:8080";

export function apiUrl(path: string) {
  return new URL(path, testApiBaseUrl).toString();
}

type ApiFixtures = {
  pullRequests?: PullRequest[];
  providers?: Provider[];
  repos?: Repo[];
  watches?: Watch[];
  watchById?: Record<number, Watch>;
  watchActivity?: Record<number, Activity[]>;
  watchOutput?: Record<number, string>;
  startedWatch?: Watch;
  stoppedWatch?: Record<number, Watch>;
  addedRepo?: Repo;
  settings?: Settings;
  savedSettings?: Settings[];
  logLevel?: LogLevel;
  savedLogLevels?: LogLevel[];
  notifications?: Notification[];
  readNotifications?: { ids?: number[] }[];
  stopBodies?: StopBody[];
  startBodies?: Record<string, unknown>[];
  settingsFail?: boolean;
  viewer?: Viewer | null;
  rateLimit?: RateLimit;
  proposals?: Record<number, Proposal[]>;
  proposalDetail?: Record<string, ProposalDetail>;
  decisions?: Decision[];
  repoConfig?: RepoConfig;
  repoConfigBodies?: RepoConfigUpdate[];
  repoConfigRefusal?: { code: string; message: string };
  queue?: QueuedPullRequest[];
};

export type Decision = { route: string; watch: number; number?: number; body: Record<string, unknown> };

export const branchRuleApprovals = 2;

export type StopBody = { keepWorktree?: boolean };

export const server = setupServer();

export const toggledOnAt = "2026-09-24T09:00:00Z";

function sinceOf(on: boolean | null | undefined, before: string | null | undefined): string | null {
  if (on === undefined || on === null) return before ?? null;
  if (!on) return null;
  return before ?? toggledOnAt;
}

function appliedConfig(current: RepoConfig, body: RepoConfigUpdate): RepoConfig {
  return {
    ...current,
    checkoutDir: body.checkoutDir ?? current.checkoutDir,
    autoStartMine: body.autoStartMine ?? current.autoStartMine,
    autoStartMineSince: sinceOf(body.autoStartMine, current.autoStartMineSince),
    includeDrafts: body.includeDrafts ?? current.includeDrafts,
    autoWatchDependabot: body.autoWatchDependabot ?? current.autoWatchDependabot,
    autoWatchDependabotSince: sinceOf(body.autoWatchDependabot, current.autoWatchDependabotSince),
    dependabotScope: body.dependabotScope ?? current.dependabotScope,
    dependabotApproval: body.dependabotApproval ?? current.dependabotApproval,
    dependabotLimit: body.dependabotLimit ?? current.dependabotLimit,
    overrides: body.overrides ?? current.overrides,
  };
}

export function serveApi(fixtures: ApiFixtures = {}) {
  setApiBaseUrl(testApiBaseUrl);
  const watchOf = (id: number) =>
    fixtures.watchById?.[id] ?? fixtures.watches?.find((item) => item.id === id) ?? buildWatch({ id });
  server.use(
    http.get(apiUrl("/api/v1/prs"), () => HttpResponse.json({ pullRequests: fixtures.pullRequests ?? [] })),
    http.get(apiUrl("/api/v1/repos"), () => HttpResponse.json({ repos: fixtures.repos ?? [] })),
    http.get(apiUrl("/api/v1/providers"), () =>
      HttpResponse.json({ providers: fixtures.providers ?? buildProviders() }),
    ),
    http.get(apiUrl("/api/v1/viewer"), () => {
      const viewer = fixtures.viewer === undefined ? buildViewer() : fixtures.viewer;
      if (!viewer) {
        return HttpResponse.json(
          { error: { code: "viewer_unavailable", message: "the daemon cannot reach GitHub" } },
          { status: 503 },
        );
      }
      return HttpResponse.json(viewer);
    }),
    http.get(apiUrl("/api/v1/ratelimit"), () =>
      HttpResponse.json(fixtures.rateLimit ?? { state: "unknown", limit: 0, remaining: 0 }),
    ),
    http.get(apiUrl("/api/v1/logs/level"), () => HttpResponse.json({ level: fixtures.logLevel ?? "info" })),
    http.put(apiUrl("/api/v1/logs/level"), async ({ request }) => {
      const body = (await request.json()) as { level: LogLevel };
      fixtures.savedLogLevels?.push(body.level);
      return HttpResponse.json(body);
    }),
    http.get(apiUrl("/api/v1/watches"), ({ request }) => {
      const status = new URL(request.url).searchParams.get("status") ?? "active";
      const watches = fixtures.watches ?? [];
      return HttpResponse.json({
        watches: status === "active" ? watches.filter((watch) => watch.status !== "stopped") : watches,
      });
    }),
    http.get(apiUrl("/api/v1/watches/:id"), ({ params }) => HttpResponse.json(watchOf(Number(params.id)))),
    http.get(apiUrl("/api/v1/watches/:id/activity"), ({ params }) => {
      const id = Number(params.id);
      return HttpResponse.json({ activity: fixtures.watchActivity?.[id] ?? [] });
    }),
    http.get(apiUrl("/api/v1/watches/:id/output"), ({ params }) => {
      const id = Number(params.id);
      return HttpResponse.json({ output: fixtures.watchOutput?.[id] ?? "" });
    }),
    http.post(apiUrl("/api/v1/watches/:id/send"), async ({ params, request }) => {
      const id = Number(params.id);
      const body = (await request.json()) as { message?: string };
      return HttpResponse.json(
        buildActivity({
          id: 900,
          watchId: id,
          kind: "nudged",
          actor: "",
          summary: `you told the agent: ${body.message ?? ""}`,
          url: "",
        }),
        { status: 201 },
      );
    }),
    http.post(apiUrl("/api/v1/watches"), async ({ request }) => {
      const body = (await request.json()) as Record<string, unknown>;
      const startedWatch =
        fixtures.startedWatch ??
        buildWatch({
          id: 99,
          repo: typeof body.repo === "string" ? body.repo : buildWatch().repo,
          number: typeof body.number === "number" ? body.number : buildWatch().number,
          title: typeof body.title === "string" ? body.title : buildWatch().title,
        });
      fixtures.startBodies?.push(body);
      return HttpResponse.json(startedWatch);
    }),
    http.post(apiUrl("/api/v1/watches/:id/stop"), async ({ params, request }) => {
      const id = Number(params.id);
      fixtures.stopBodies?.push((await request.json()) as StopBody);
      return HttpResponse.json(fixtures.stoppedWatch?.[id] ?? buildWatch({ id, status: "stopped" }));
    }),
    http.post(apiUrl("/api/v1/watches/:id/poll"), () => HttpResponse.json({})),
    http.get(apiUrl("/api/v1/watches/:id/proposals"), ({ params }) =>
      HttpResponse.json({ proposals: fixtures.proposals?.[Number(params.id)] ?? [] }),
    ),
    http.get(apiUrl("/api/v1/watches/:id/proposals/:number"), ({ params, request }) => {
      const commit = new URL(request.url).searchParams.get("commit");
      const key = `${String(params.id)}/${String(params.number)}`;
      const detail = fixtures.proposalDetail?.[commit ? `${key}@${commit}` : key];
      if (!detail) {
        const code = commit ? "commit_not_found" : "proposal_not_found";
        return HttpResponse.json({ error: { code, message: code.replaceAll("_", " ") } }, { status: 404 });
      }
      return HttpResponse.json(detail);
    }),
    ...["approve", "reject", "retry"].map((route) =>
      http.post(apiUrl(`/api/v1/watches/:id/proposals/:number/${route}`), async ({ params, request }) => {
        const text = await request.text();
        const body = text ? (JSON.parse(text) as Record<string, unknown>) : {};
        const watch = Number(params.id);
        const number = Number(params.number);
        fixtures.decisions?.push({ route, watch, number, body });
        const proposal = fixtures.proposals?.[watch]?.find((p) => p.number === number);
        const status = route === "reject" ? "rejected" : "released";
        return HttpResponse.json({ ...proposal, number, status, pushRejected: body.rejectPush === true });
      }),
    ),
    http.post(apiUrl("/api/v1/watches/:id/approval"), async ({ params, request }) => {
      const body = (await request.json()) as Record<string, unknown>;
      const watch = Number(params.id);
      fixtures.decisions?.push({ route: "approval", watch, body });
      const current = watchOf(watch);
      return HttpResponse.json({
        ...current,
        approvalMode: body.mode ?? current.approvalMode,
        pendingProposal: undefined,
      });
    }),
    http.patch(apiUrl("/api/v1/watches/:id"), async ({ params, request }) => {
      const body = (await request.json()) as {
        approvalsRequired?: number | null;
        mergeMethod?: string;
        mergeWhenReady?: boolean;
      };
      const watch = Number(params.id);
      fixtures.decisions?.push({ route: "update", watch, body });
      const current = watchOf(watch);
      const approvalsRequired =
        body.approvalsRequired === null ? branchRuleApprovals : (body.approvalsRequired ?? current.approvalsRequired);
      return HttpResponse.json({
        ...current,
        approvalsRequired,
        mergeMethod: body.mergeMethod ?? current.mergeMethod,
        mergeWhenReady: body.mergeWhenReady ?? current.mergeWhenReady,
      });
    }),
    http.post(apiUrl("/api/v1/watches/:id/merge"), async ({ params, request }) => {
      const body = (await request.json()) as Record<string, unknown>;
      const watch = Number(params.id);
      fixtures.decisions?.push({ route: "merge", watch, body });
      return HttpResponse.json({ ...watchOf(watch), status: "stopped", stopReason: "merged" });
    }),
    http.post(apiUrl("/api/v1/repos"), async ({ request }) => {
      const body = (await request.json()) as { fullName?: string };
      return HttpResponse.json(fixtures.addedRepo ?? buildRepo({ fullName: body.fullName ?? buildRepo().fullName }));
    }),
    http.delete(apiUrl("/api/v1/repos/:id"), () => new HttpResponse(null, { status: 204 })),
    http.get(apiUrl("/api/v1/repos/:id/config"), ({ params }) =>
      HttpResponse.json(fixtures.repoConfig ?? buildRepoConfig({ repoId: Number(params.id) })),
    ),
    http.patch(apiUrl("/api/v1/repos/:id/config"), async ({ params, request }) => {
      const body = (await request.json()) as RepoConfigUpdate;
      fixtures.repoConfigBodies?.push(body);
      if (fixtures.repoConfigRefusal) {
        return HttpResponse.json({ error: fixtures.repoConfigRefusal }, { status: 400 });
      }
      const current = fixtures.repoConfig ?? buildRepoConfig({ repoId: Number(params.id) });
      fixtures.repoConfig = appliedConfig(current, body);
      return HttpResponse.json(fixtures.repoConfig);
    }),
    http.get(apiUrl("/api/v1/repos/:id/queue"), () =>
      HttpResponse.json({ repo: buildRepo().fullName, pullRequests: fixtures.queue ?? [] }),
    ),
    http.post(apiUrl("/api/v1/sync"), () => HttpResponse.json({ accepted: true }, { status: 202 })),
    http.get(apiUrl("/api/v1/notifications"), ({ request }) => {
      const unreadOnly = new URL(request.url).searchParams.get("status") === "unread";
      const all = fixtures.notifications ?? [];
      const notifications = unreadOnly ? all.filter((item) => !item.readAt) : all;
      return HttpResponse.json({ notifications, unreadCount: all.filter((item) => !item.readAt).length });
    }),
    http.post(apiUrl("/api/v1/notifications/read"), async ({ request }) => {
      const body = (await request.json()) as { ids?: number[] };
      fixtures.readNotifications?.push(body);
      for (const item of fixtures.notifications ?? []) {
        if (!body.ids || body.ids.includes(item.id)) item.readAt = "2026-09-21T12:30:00Z";
      }
      return HttpResponse.json({ unreadCount: (fixtures.notifications ?? []).filter((item) => !item.readAt).length });
    }),
    http.get(apiUrl("/api/v1/settings"), () => {
      if (fixtures.settingsFail)
        return HttpResponse.json({ error: { code: "boom", message: "no settings" } }, { status: 500 });
      return HttpResponse.json(fixtures.settings ?? buildSettings());
    }),
    http.put(apiUrl("/api/v1/settings"), async ({ request }) => {
      const body = (await request.json()) as Settings;
      fixtures.savedSettings?.push(body);
      return HttpResponse.json(body);
    }),
  );
}
