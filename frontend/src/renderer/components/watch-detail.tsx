import { useMemo, useState, type FormEvent, type ReactNode } from "react";
import {
  BotIcon,
  CheckCircle2Icon,
  ChevronDownIcon,
  CircleAlertIcon,
  EyeIcon,
  FileDiffIcon,
  GitBranchIcon,
  GitCommitHorizontalIcon,
  GitMergeIcon,
  HeartPulseIcon,
  MessageSquareIcon,
  MessageSquareReplyIcon,
  PanelRightIcon,
  PlayIcon,
  PowerOffIcon,
  RefreshCwIcon,
  SendIcon,
  ShieldCheckIcon,
  ShieldQuestionMarkIcon,
  SquareIcon,
  TerminalIcon,
  TriangleAlertIcon,
  Undo2Icon,
  XCircleIcon,
  ZapIcon,
  type LucideIcon,
} from "lucide-react";
import type { Proposal } from "@/hooks/useProposals";
import { useResizeTerminal, useSendMessage, useWatchOutput } from "@/hooks/useSession";
import { ProposalPanel } from "@/components/proposal-panel";
import { ProposalDecisionProvider, useCurrentProposal, useProposalDecision } from "@/components/proposal-decision";
import { WatchSettingsPanel } from "@/components/watch-settings-panel";
import { useWatchActivity, type Activity } from "@/hooks/useWatchActivity";
import { usePollWatch, useWatch, useWatches, type Watch } from "@/hooks/useWatches";
import { AgentTerminal } from "@/components/agent-terminal";
import {
  AutoBadges,
  ChecksBadge,
  MergeableBadge,
  MergeBadge,
  Meta,
  SessionBadge,
  StopBadge,
  ToneBadge,
} from "@/components/status-badges";
import { ViewHeader } from "@/components/view-header";
import { MergeWatchDialog } from "@/components/merge-watch-dialog";
import { StopWatchDialog } from "@/components/stop-watch-dialog";
import { TakenOverPanel } from "@/components/taken-over-panel";
import { TakeoverDialog } from "@/components/takeover-dialog";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import type { Navigate } from "@/lib/navigation";
import { duration, relativeTime, shortSha } from "@/lib/time";
import {
  activityUrl,
  checkRows,
  heldForHandback,
  isSelfWatch,
  isTakenOver,
  mergeMethodText,
  sessionWord,
  stopReasonText,
  watchLabel,
  worktreeText,
} from "@/lib/watch-status";
import { cn } from "@/lib/utils";

const icons: Record<Activity["kind"], { icon: LucideIcon; tone?: string }> = {
  comment: { icon: MessageSquareIcon },
  review_comment: { icon: MessageSquareIcon },
  review: { icon: MessageSquareIcon },
  check_failed: { icon: XCircleIcon, tone: "text-destructive" },
  check_recovered: { icon: CheckCircle2Icon },
  checks_green: { icon: CheckCircle2Icon, tone: "text-success" },
  commit: { icon: GitCommitHorizontalIcon },
  behind: { icon: GitBranchIcon, tone: "text-chart-3" },
  conflict: { icon: TriangleAlertIcon, tone: "text-chart-3" },
  merged: { icon: GitMergeIcon, tone: "text-chart-4" },
  closed: { icon: XCircleIcon },
  heartbeat: { icon: HeartPulseIcon },
  watch_started: { icon: PlayIcon },
  watch_stopped: { icon: SquareIcon },
  session_started: { icon: BotIcon, tone: "text-chart-4" },
  session_exited: { icon: PowerOffIcon, tone: "text-chart-3" },
  nudged: { icon: SendIcon, tone: "text-attention" },
  agent_failed: { icon: CircleAlertIcon, tone: "text-destructive" },
  merge_ready: { icon: GitMergeIcon, tone: "text-success" },
  merge_failed: { icon: CircleAlertIcon, tone: "text-destructive" },
  replied: { icon: MessageSquareReplyIcon },
  review_requested: { icon: EyeIcon },
  proposal: { icon: FileDiffIcon, tone: "text-attention" },
  taken_over: { icon: TerminalIcon },
  handed_back: { icon: Undo2Icon },
  auto_started: { icon: ZapIcon },
  approved: { icon: ShieldCheckIcon, tone: "text-success" },
  approval_asked: { icon: ShieldQuestionMarkIcon, tone: "text-attention" },
};

const checkTone: Record<string, string> = {
  failed: "text-destructive",
  passed: "text-muted-foreground",
  success: "text-muted-foreground",
  pending: "text-chart-3",
};

function messageOf(a: Activity): string | null {
  const message = a.payload?.message;
  return typeof message === "string" && message ? message : null;
}

type Props = {
  id: number;
  enabled: boolean;
  onNavigate: Navigate;
  onStopped: (watch: Watch) => void;
  onWatchPR: () => void;
};

export function WatchDetail({ id, enabled, onStopped, onWatchPR }: Props) {
  const listed = useWatches(enabled, "all");
  const one = useWatch(enabled ? id : null);
  const watch = listed.data?.find((w) => w.id === id) ?? one.data;
  const activity = useWatchActivity(enabled ? id : null);
  const poll = usePollWatch();
  const current = useCurrentProposal(watch);
  const [stopping, setStopping] = useState(false);
  const [merging, setMerging] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const rows = useMemo(() => (activity.data ? [...activity.data].reverse() : []), [activity.data]);
  const checks = useMemo(() => (watch ? checkRows(watch) : []), [watch]);

  if (!watch) {
    if (one.error) {
      return (
        <>
          <ViewHeader />
          <div className="p-5">
            <Alert variant="destructive">
              <CircleAlertIcon />
              <AlertTitle>{one.error.message}</AlertTitle>
            </Alert>
          </div>
        </>
      );
    }
    return (
      <>
        <ViewHeader>
          <Skeleton className="h-5 w-72" />
        </ViewHeader>
        <div className="flex flex-col gap-3 p-5">
          <Skeleton className="h-5 w-96" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      </>
    );
  }

  const active = watch.status === "active";
  const ready = active && Boolean(watch.readySince);
  const blockers = active ? (watch.readyBlockers ?? []) : [];

  return (
    <ProposalDecisionProvider key={watch.id} watch={watch}>
      <div className="flex min-h-0 flex-1">
        <div className="flex min-w-0 flex-1 flex-col overflow-y-auto">
          <ViewHeader className="flex-col items-stretch gap-2 bg-muted-subtle pb-3.5">
            <div className="flex h-titlebar items-center gap-2">
              <h1 className={cn("truncate text-lg font-medium tracking-tight", !active && "text-foreground/75")}>
                {watch.title || watchLabel(watch)}
              </h1>
              <Meta className="shrink-0">
                <a href={watch.url} target="_blank" rel="noreferrer" className="hover:underline">
                  {watchLabel(watch)}
                </a>
              </Meta>
              {active ? (
                <div className="ml-auto flex shrink-0 items-center gap-1.5">
                  <Button variant="ghost" size="sm" disabled={poll.isPending} onClick={() => poll.mutate(id)}>
                    {poll.isPending ? <Spinner data-icon="inline-start" /> : <RefreshCwIcon data-icon="inline-start" />}
                    Check now
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setStopping(true)}>
                    Stop watching
                  </Button>
                  <Button
                    size="sm"
                    disabled={!ready}
                    title={ready ? undefined : "The pull request is not ready to merge yet"}
                    onClick={() => setMerging(true)}
                  >
                    <GitMergeIcon data-icon="inline-start" />
                    Merge
                  </Button>
                  {settingsOpen ? null : (
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Watch settings"
                      title="Watch settings"
                      className="size-7"
                      onClick={() => setSettingsOpen(true)}
                    >
                      <PanelRightIcon />
                    </Button>
                  )}
                </div>
              ) : null}
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {!active ? <StopBadge watch={watch} className="shrink-0" /> : null}
              <ChecksBadge watch={watch} />
              <MergeableBadge state={watch.mergeableState} />
              <MergeBadge watch={watch} />
              {isTakenOver(watch) ? <ToneBadge tone="neutral">with you</ToneBadge> : null}
              <AutoBadges watch={watch} />
              <Meta>
                {watch.headRef} · {shortSha(watch.headSha)}
              </Meta>
              <Meta>
                {active
                  ? `watching ${duration(watch.startedAt)}`
                  : `watched ${duration(watch.startedAt, watch.stoppedAt)}`}
              </Meta>
              {watch.lastPollAt ? <Meta>checked {relativeTime(watch.lastPollAt)}</Meta> : null}
              {watch.lastError ? (
                <Badge variant="destructive" title={watch.lastError}>
                  error
                </Badge>
              ) : null}
            </div>
            {poll.error ? (
              <Alert variant="destructive">
                <CircleAlertIcon />
                <AlertTitle>{poll.error.message}</AlertTitle>
              </Alert>
            ) : null}
          </ViewHeader>

          {ready ? (
            <div className="flex flex-wrap items-center gap-2 border-b bg-success/5 px-5 py-2.5 text-sm">
              <GitMergeIcon className="size-4 shrink-0 text-success" />
              <span>
                Ready to merge since {relativeTime(watch.readySince ?? "")}. {readyText(watch)}
              </span>
            </div>
          ) : null}
          {!ready && blockers.length > 0 ? (
            <div className="flex flex-col gap-1 border-b px-5 py-2.5">
              <span className="eyebrow">Not ready to merge</span>
              <ul aria-label="What blocks the merge" className="flex flex-col gap-0.5 text-sm text-foreground/80">
                {blockers.map((b) => (
                  <li key={b}>{b}</li>
                ))}
              </ul>
            </div>
          ) : null}

          <StopWatchDialog open={stopping} onOpenChange={setStopping} watch={watch} onStopped={onStopped} />
          <MergeWatchDialog open={merging} onOpenChange={setMerging} watch={watch} onMerged={onStopped} />

          {active ? <ProposalPanel watch={watch} /> : null}

          {!active ? <ArchiveSummary watch={watch} onWatchPR={onWatchPR} /> : null}

          {isSelfWatch(watch) ? <SelfSessionPanel watch={watch} /> : <SessionPanel watch={watch} enabled={enabled} />}

          {checks.length > 0 ? (
            <Collapsible className="border-b">
              <CollapsibleTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-start rounded-none px-5 font-mono text-xs text-muted-foreground"
                >
                  <ChevronDownIcon
                    data-icon="inline-start"
                    className="transition-transform [[data-state=open]>&]:rotate-180"
                  />
                  {checks.length} {checks.length === 1 ? "check" : "checks"} on {shortSha(watch.headSha)}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent>
                {checks.map((row) => (
                  <div key={row.name} className="flex items-center gap-3 border-t px-5 py-1.5 text-sm">
                    <span
                      className={cn("w-16 shrink-0 font-mono text-xs", checkTone[row.state] ?? "text-muted-foreground")}
                    >
                      {row.state}
                    </span>
                    <span className="truncate text-foreground/80">{row.name}</span>
                  </div>
                ))}
              </CollapsibleContent>
            </Collapsible>
          ) : null}

          {activity.error ? (
            <div className="p-5">
              <Alert variant="destructive">
                <CircleAlertIcon />
                <AlertTitle>{activity.error.message}</AlertTitle>
              </Alert>
            </div>
          ) : null}

          {activity.isPending ? (
            <div className="flex flex-col gap-3 p-5">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : null}

          {rows.map((a) => {
            const { icon: Icon, tone } = icons[a.kind] ?? {
              icon: MessageSquareIcon,
            };
            const failed = a.kind === "agent_failed";
            const retry = active ? retryOffered(a, current) : null;
            const heartbeat = a.kind === "heartbeat";
            const message = a.kind === "nudged" ? messageOf(a) : null;
            return (
              <div
                key={a.id}
                className={cn(
                  "flex items-start gap-3 border-b px-5 py-2.5",
                  failed && "bg-destructive/5",
                  heartbeat && "text-muted-foreground/70",
                )}
              >
                <Icon className={cn("mt-0.5 size-4 shrink-0", heartbeat ? "text-muted-foreground/50" : tone)} />
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  {a.url && a.kind !== "nudged" ? (
                    <a
                      href={activityUrl(a)}
                      target="_blank"
                      rel="noreferrer"
                      className={cn("wrap-break-word hover:underline", heartbeat ? "text-body" : "text-sm")}
                    >
                      {a.summary}
                    </a>
                  ) : (
                    <span className={cn("wrap-break-word", heartbeat ? "text-body" : "text-sm")}>{a.summary}</span>
                  )}
                  <Meta className={cn(heartbeat && "text-muted-foreground/60")}>
                    {a.kind.replaceAll("_", " ")}
                    {a.payload?.by === "daemon" ? " · daemon" : ""}
                    {a.payload?.by === "author" ? " · you" : ""}
                    {a.actor ? ` · ${a.actor}` : ""} · {relativeTime(a.at)}
                    {a.nudgedAt ? " · told the agent" : ""}
                    {heldForHandback(watch, a) ? " · held for the hand-back" : ""}
                  </Meta>
                  {retry !== null ? <RetryButton number={retry} /> : null}
                  {message ? (
                    <Collapsible>
                      <CollapsibleTrigger asChild>
                        <Button variant="ghost" size="xs" className="-ml-2 font-mono text-2xs text-muted-foreground">
                          <ChevronDownIcon
                            data-icon="inline-start"
                            className="transition-transform [[data-state=open]>&]:rotate-180"
                          />
                          the message
                        </Button>
                      </CollapsibleTrigger>
                      <CollapsibleContent>
                        <pre className="mt-1 max-h-72 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-2xs/relaxed whitespace-pre-wrap">
                          {message}
                        </pre>
                      </CollapsibleContent>
                    </Collapsible>
                  ) : null}
                </div>
              </div>
            );
          })}

          {activity.data && activity.data.length === 0 ? (
            <Empty className="py-12">
              <EmptyHeader>
                <EmptyTitle>Nothing yet</EmptyTitle>
                <EmptyDescription>
                  Quiet checks stay silent. New comments, checks and commits show here, and so does every message to the
                  agent.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : null}
        </div>
        {active && settingsOpen ? <WatchSettingsPanel watch={watch} onClose={() => setSettingsOpen(false)} /> : null}
      </div>
    </ProposalDecisionProvider>
  );
}

function readyText(watch: Watch): string {
  if (watch.mergeWhenReady) {
    return `Merge when ready is on, so the daemon merges it now with ${mergeMethodText(watch.mergeMethod)}.`;
  }
  return "Merge it when it suits you. Merge when ready is off, so the daemon does not merge it.";
}

function retryOffered(a: Activity, current: Proposal | undefined): number | null {
  const failedRelease = a.kind === "agent_failed" && Boolean(a.payload?.retry);
  const stillFailed = current?.status === "failed" && a.payload?.proposal === current.number;
  return failedRelease && stillFailed ? current.number : null;
}

function RetryButton({ number }: { number: number }) {
  const decision = useProposalDecision();
  const retry = decision.retrying;
  return (
    <div className="flex items-center gap-2">
      <Button
        variant="outline"
        size="xs"
        className="w-fit"
        disabled={retry.isPending}
        onClick={() => decision.retry(number)}
      >
        {retry.isPending ? <Spinner data-icon="inline-start" /> : <RefreshCwIcon data-icon="inline-start" />}
        Retry
      </Button>
      {retry.error ? <span className="text-xs text-destructive">{retry.error.message}</span> : null}
    </div>
  );
}

function AgentHeader({ badge, children }: { badge: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <BotIcon className="size-4 shrink-0 text-muted-foreground" />
      <span className="text-sm font-medium">Agent</span>
      {badge}
      {children}
    </div>
  );
}

function SelfSessionPanel({ watch }: { watch: Watch }) {
  return (
    <div className="flex flex-col gap-1.5 border-b px-5 py-3.5">
      <AgentHeader badge={<ToneBadge tone="neutral">your own session</ToneBadge>}>
        <Meta>{watch.provider}</Meta>
      </AgentHeader>
      <p className="text-sm text-foreground/75">
        The coding session that started this watch is its agent, in {watch.sourceDir}.
        {watch.status === "active" ? (
          <>
            {" "}
            It takes each message with{" "}
            <code className="font-mono text-xs/normal">babysitter watch next {watch.id}</code>.
          </>
        ) : null}{" "}
        That session pushes and replies itself, so this watch has no approval gate.
      </p>
    </div>
  );
}

function SessionPanel({ watch, enabled }: { watch: Watch; enabled: boolean }) {
  const active = watch.status === "active";
  const output = useWatchOutput(enabled ? watch.id : null);
  const send = useSendMessage();
  const resize = useResizeTerminal();
  const [message, setMessage] = useState("");
  const [open, setOpen] = useState(false);
  const [takingOver, setTakingOver] = useState(false);
  const session = sessionWord(watch.session.state);
  const held = Boolean(watch.pendingProposal);
  const withYou = isTakenOver(watch);
  const offersTakeover = active && !withYou && Boolean(watch.agentSession || watch.session.state !== "none");
  const terminalWord = withYou ? "the last session" : "the terminal";

  function submit(event: FormEvent) {
    event.preventDefault();
    const trimmed = message.trim();
    if (!trimmed) return;
    send.mutate({ id: watch.id, message: trimmed }, { onSuccess: () => setMessage("") });
  }

  return (
    <div className="flex flex-col gap-2.5 border-b px-5 py-3.5">
      <AgentHeader
        badge={withYou ? <ToneBadge tone="neutral">with you</ToneBadge> : <SessionBadge state={watch.session.state} />}
      >
        <Meta>
          {watch.provider}
          {watch.model ? ` · ${watch.model}` : ""}
          {watch.effort ? ` · ${watch.effort} effort` : ""}
        </Meta>
        {withYou ? <Meta>taken over {relativeTime(watch.takenOverAt ?? "")}</Meta> : null}
        {!withYou && watch.session.pid > 0 ? <Meta>pid {watch.session.pid}</Meta> : null}
        {!withYou && watch.session.signalAt ? <Meta>heard {relativeTime(watch.session.signalAt)}</Meta> : null}
        <div className="ml-auto flex items-center gap-1.5">
          {offersTakeover ? (
            <Button variant="outline" size="xs" onClick={() => setTakingOver(true)}>
              <TerminalIcon data-icon="inline-start" />
              Continue in terminal
            </Button>
          ) : null}
          <Button
            variant="ghost"
            size="xs"
            className="font-mono text-2xs text-muted-foreground"
            onClick={() => setOpen((v) => !v)}
          >
            <ChevronDownIcon data-icon="inline-start" className={cn("transition-transform", open && "rotate-180")} />
            {open ? `hide ${terminalWord}` : `show ${terminalWord}`}
          </Button>
        </div>
      </AgentHeader>
      <TakeoverDialog open={takingOver} onOpenChange={setTakingOver} watch={watch} />
      {held && active && !withYou ? (
        <p className="text-sm text-muted-foreground">
          A proposal waits on you, so nothing reaches the agent until you decide. To have it try again, reject the
          proposal and say why.
        </p>
      ) : null}
      {session.needsYou && active && !withYou ? (
        <p className="text-sm text-attention">
          {watch.session.state === "exited"
            ? "The agent process ended. It starts again with the next message."
            : "The agent waits on you. Read what it printed and answer it below."}
        </p>
      ) : null}
      {open ? (
        output.error || !output.data ? (
          <pre
            aria-label="Agent output"
            className="max-h-96 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-2xs/relaxed whitespace-pre-wrap"
          >
            {output.error ? output.error.message : "Nothing printed yet."}
          </pre>
        ) : (
          <AgentTerminal
            output={output.data}
            onResize={active ? (grid) => resize.mutate({ id: watch.id, ...grid }) : undefined}
          />
        )
      ) : null}
      {withYou ? <TakenOverPanel watch={watch} /> : null}
      {active && !withYou ? (
        <form onSubmit={submit} className="flex items-start gap-2">
          <textarea
            aria-label="Message to the agent"
            value={message}
            onChange={(event) => setMessage(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) submit(event);
            }}
            rows={2}
            disabled={held}
            placeholder={
              held
                ? "The agent takes no message while a proposal waits."
                : "Tell the agent something. It is typed into its session as if you had."
            }
            className="min-h-9 flex-1 resize-y rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          />
          <Button
            type="submit"
            size="sm"
            disabled={held || send.isPending || !message.trim() || watch.session.state === "blocked"}
          >
            {send.isPending ? <Spinner data-icon="inline-start" /> : <SendIcon data-icon="inline-start" />}
            Send
          </Button>
        </form>
      ) : null}
      {send.error ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{send.error.message}</AlertTitle>
        </Alert>
      ) : null}
    </div>
  );
}

function Tile({ value, label }: { value: number; label: string }) {
  return (
    <div className="flex flex-1 flex-col gap-0.5 rounded-md border px-2.5 py-2">
      <span className="text-xl font-medium">{value}</span>
      <span className="eyebrow">{label}</span>
    </div>
  );
}

function ArchiveSummary({ watch, onWatchPR }: { watch: Watch; onWatchPR: () => void }) {
  const sum = watch.summary;
  const activity = sum?.activity ?? {};
  const comments = (activity.comment ?? 0) + (activity.review_comment ?? 0) + (activity.review ?? 0);
  return (
    <div className="flex flex-col gap-3 border-b px-5 py-4">
      <p className="text-title font-medium">
        {stopReasonText(watch.stopReason)} Here is what happened while you were away.
      </p>
      {sum ? (
        <div className="flex gap-2.5">
          <Tile value={sum.messages} label="messages to the agent" />
          <Tile value={comments} label="review items" />
          <Tile value={activity.commit ?? 0} label="commits" />
          <Tile value={activity.check_failed ?? 0} label="failed checks" />
        </div>
      ) : null}
      {sum?.detail ? <p className="text-sm text-foreground/75">{sum.detail}</p> : null}
      <div className="flex items-center gap-3 border-t pt-3">
        <span className="text-sm text-foreground/75">The worktree is {worktreeText(watch)}.</span>
        <Button variant="outline" size="sm" className="ml-auto shrink-0" onClick={onWatchPR}>
          Watch another PR
        </Button>
      </div>
    </div>
  );
}
