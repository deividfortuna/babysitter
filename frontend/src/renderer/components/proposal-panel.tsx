import { useMemo, useState, type ComponentProps } from "react";
import {
  CheckIcon,
  ChevronDownIcon,
  CircleXIcon,
  FileDiffIcon,
  MessageSquareReplyIcon,
  RotateCcwIcon,
  Trash2Icon,
  TriangleAlertIcon,
  Undo2Icon,
} from "lucide-react";
import {
  useProposal,
  useProposalFiles,
  type FileLoad,
  type Proposal,
  type ProposalDetail,
  type ProposalReply,
} from "@/hooks/useProposals";
import type { Watch } from "@/hooks/useWatches";
import { AttentionBadge, Meta, ToneBadge } from "@/components/status-badges";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ButtonGroup, ButtonGroupSeparator } from "@/components/ui/button-group";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import {
  commitsWord,
  count,
  DropReplyDialog,
  RejectProposalDialog,
  RejectPushDialog,
  StopAskingDialog,
  type Outgoing,
} from "@/components/proposal-dialogs";
import { outgoing, useProposalDecision, type Preview } from "@/components/proposal-decision";
import { CodeArea, type RenderReply, type ShownCode } from "@/components/proposal-code";
import type { MissingLoad } from "@/components/proposal-file-tree";
import { anchorReplies, readPatch, readProposalDiff, type AnchoredReplies } from "@/lib/proposal-diff";
import { relativeTime, shortSha } from "@/lib/time";
import { cn } from "@/lib/utils";

function conflicted(p: Proposal): boolean {
  return p.status === "failed" && (p.error ?? "").includes(" conflicts in ");
}

function failure(p: Proposal): string {
  return (p.error ?? "").replace(/^[^:]*proposal \d+: /, "");
}

export function ProposalPanel({ watch }: { watch: Watch }) {
  const { current, outcome } = useProposalDecision();

  if (current && current.number !== outcome?.number) {
    if (conflicted(current)) return <ConflictSection watch={watch} proposal={current} />;
    if (current.status === "failed") return <FailedSection watch={watch} proposal={current} />;
    return <PendingSection key={current.number} watch={watch} proposal={current} />;
  }
  if (!outcome) return null;
  const Icon = outcome.icon === "released" ? CheckIcon : CircleXIcon;
  return (
    <div
      role="status"
      aria-label="Decision"
      className={cn(
        "flex items-center gap-2 border-b px-5 py-2.5 text-sm",
        outcome.icon === "released" && "bg-success/5",
      )}
    >
      <Icon className={cn("size-4 shrink-0", outcome.icon === "released" ? "text-success" : "text-muted-foreground")} />
      <span>{outcome.text}</span>
    </div>
  );
}

function Heading({ proposal, children }: { proposal: Proposal; children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <FileDiffIcon className="size-4 shrink-0 text-attention" />
      <h2 className="text-title font-medium">Proposal {proposal.number}</h2>
      {children}
    </div>
  );
}

function PendingSection({ watch, proposal }: { watch: Watch; proposal: Proposal }) {
  const decision = useProposalDecision();
  const { draft, change, detail, preview, codeUnread, approving, rejecting: rejectRequest } = decision;
  const { edits, dropped, pushRejected } = draft;
  const [dropping, setDropping] = useState<ProposalReply | null>(null);
  const [rejectingPush, setRejectingPush] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [stopAsking, setStopAsking] = useState(false);

  const replies = useMemo(() => detail?.replies ?? proposal.replies ?? [], [detail?.replies, proposal.replies]);
  const code = preview.ready ? preview.detail : undefined;
  const commits = useMemo(() => code?.commits ?? [], [code?.commits]);
  const [picked, setPicked] = useState<{ work: string; sha: string } | null>(null);
  const commit = picked?.work === proposal.workSha ? picked.sha : null;
  const shown = useShownCode(watch.id, proposal, code, commit);
  const pushes = proposal.hasPush && !pushRejected;
  const inline = pushes && shown.base === proposal.headSha;
  const anchored = useMemo(
    (): AnchoredReplies =>
      shown.diff && inline
        ? anchorReplies(replies, shown.diff.files, proposal.headSha)
        : { annotations: new Map(), unanchored: replies },
    [shown.diff, inline, replies, proposal.headSha],
  );
  const inDiff = [...anchored.annotations.values()].flat().filter((a) => !dropped.includes(a.metadata.replyId)).length;
  const placing = inline && !shown.diff && !preview.error && !shown.error;
  const bot = watch.dependabot;
  const out = decision.out ?? outgoing(watch, proposal, detail, draft);
  const heldBack = commits.filter((c) => c.heldBack).length;

  const actions = (
    <DecisionButtons
      hasPush={proposal.hasPush}
      pushes={pushes}
      approving={approving.isPending}
      codeUnread={codeUnread}
      onApprove={() => decision.approve(proposal.number, false)}
      onStopAsking={() => setStopAsking(true)}
      onReject={() => setRejecting(true)}
      onRejectPush={() => setRejectingPush(true)}
    />
  );
  const failed = approving.error ?? rejectRequest.error;
  const refusal = failed ? (
    <Alert variant="destructive">
      <CircleXIcon />
      <AlertTitle>{failed.message}</AlertTitle>
    </Alert>
  ) : null;

  const renderReply: RenderReply = (r, anchor) =>
    dropped.includes(r.id) ? (
      <div key={r.id} className="flex items-center gap-2 text-sm text-muted-foreground">
        <Trash2Icon className="size-4 shrink-0" />
        <span>
          {replyWho(r).label} dropped. Nothing is posted
          {r.inReplyTo ? ", and the comment reaches the agent again" : ""}.
        </span>
        <Button
          variant="ghost"
          size="xs"
          onClick={() => change((d) => ({ ...d, dropped: d.dropped.filter((id) => id !== r.id) }))}
        >
          Undo
        </Button>
      </div>
    ) : (
      <ReplyCard
        key={r.id}
        reply={r}
        note={anchor && !anchor.placed ? `On line ${anchor.line}, which this change does not show.` : undefined}
        value={edits[r.id] ?? r.edited ?? r.body}
        edited={edits[r.id] !== undefined && edits[r.id] !== r.body}
        onChange={(text) => change((d) => ({ ...d, edits: { ...d.edits, [r.id]: text } }))}
        onDrop={() => setDropping(r)}
      />
    );

  const moved = proposal.rebasedFrom ? (proposal.movedBy === "merge" ? "merged" : "rebased") : undefined;
  const movedIntro =
    moved === "merged"
      ? "The pull request branch moved while this waited, and the daemon merged it into the work without conflicts. The commits you were reading stay as they were, with a merge commit on top."
      : "The pull request branch moved while this waited, and the daemon rebased the work onto it without conflicts. It is the change you were reading, on new commits, and a reply that named an old commit now names the new one.";
  const intro = bot
    ? "The agent answered a comment. Dependabot owns this branch, so the proposal carries replies only: the daemon pushes nothing and rebases nothing."
    : moved
      ? movedIntro
      : "The agent finished a turn. None of it is on GitHub yet: what you approve is what goes out, under your account.";

  return (
    <section aria-label="Proposal" className="flex flex-col gap-3 border-b px-5 py-3.5">
      <Heading proposal={proposal}>
        <AttentionBadge>approval needed</AttentionBadge>
        {moved ? (
          <Badge variant="outline" className="font-mono">
            {moved}
          </Badge>
        ) : null}
        <Meta>
          {moved ?? "opened"} {relativeTime((moved ? proposal.endedAt : proposal.openedAt) ?? proposal.openedAt)}
        </Meta>
        <Meta>head {shortSha(proposal.headSha)}</Meta>
        {proposal.hasPush ? (
          <Meta>
            work {shortSha(proposal.workSha)}
            {proposal.rebasedFrom ? `, was ${shortSha(proposal.rebasedFrom)}` : ""}
          </Meta>
        ) : null}
        <div className="ml-auto">{actions}</div>
      </Heading>
      <p className="text-sm text-foreground/80">{intro}</p>
      {heldBack > 0 ? (
        <p className="text-sm text-attention">
          {heldBack === 1
            ? "1 commit here is one you kept off the pull request in an earlier decision. Approving pushes it with the rest."
            : `${heldBack} commits here are ones you kept off the pull request in an earlier decision. Approving pushes them with the rest.`}
        </p>
      ) : null}

      {pushRejected ? (
        <div className="flex items-center gap-2 rounded-md border border-destructive/60 px-3 py-2 text-sm">
          <TriangleAlertIcon className="size-4 shrink-0 text-destructive" />
          <span>Push rejected. Approving posts the replies only; the commits stay on the work branch.</span>
          <Button
            variant="ghost"
            size="xs"
            className="ml-auto"
            onClick={() => change((d) => ({ ...d, pushRejected: false }))}
          >
            <Undo2Icon data-icon="inline-start" />
            Restore the push
          </Button>
        </div>
      ) : null}

      {refusal}

      {proposal.hasPush ? (
        <ProposalCode
          preview={preview}
          shown={shown}
          commits={commits}
          commit={commit}
          onCommit={(sha) => setPicked(sha ? { work: proposal.workSha, sha } : null)}
          annotations={anchored.annotations}
          replies={replies}
          renderReply={renderReply}
          dimmed={pushRejected}
          title={`proposal ${proposal.number} · ${watch.repo}#${watch.number}`}
          viewedKey={`${watch.id}:${proposal.number}`}
          actions={actions}
          notice={refusal}
        />
      ) : null}

      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        {replies.length > 0 && !placing ? (
          <span className="eyebrow">
            {count(replies.length - dropped.length, "reply", "replies")} · posted under your account
            {dropped.length > 0 ? ` · ${dropped.length} dropped` : ""}
            {inDiff > 0 ? ` · ${inDiff} in the diff` : ""}
          </span>
        ) : null}
        <Meta className="ml-auto">{noteLine(out, proposal)}</Meta>
      </div>
      {anchored.unanchored.length > 0 && !placing ? (
        <div className="flex flex-col gap-2.5">{anchored.unanchored.map((r) => renderReply(r))}</div>
      ) : null}

      <DropReplyDialog
        open={dropping !== null}
        onOpenChange={(open) => !open && setDropping(null)}
        who={dropping ? replyWho(dropping).who : ""}
        answers={Boolean(dropping?.inReplyTo)}
        text={dropping ? (edits[dropping.id] ?? dropping.body) : ""}
        onConfirm={() => {
          if (dropping) change((d) => ({ ...d, dropped: [...d.dropped, dropping.id] }));
          setDropping(null);
        }}
      />
      <RejectPushDialog
        open={rejectingPush}
        onOpenChange={setRejectingPush}
        number={proposal.number}
        commits={preview.ready ? (detail?.commits ?? []).map((c) => c.sha) : null}
        replies={out.replies}
        headRef={watch.headRef}
        onConfirm={() => {
          change((d) => ({ ...d, pushRejected: true }));
          setRejectingPush(false);
        }}
      />
      <RejectProposalDialog
        open={rejecting}
        onOpenChange={setRejecting}
        watch={watch}
        number={proposal.number}
        head={proposal.headSha}
        pending={rejectRequest.isPending}
        error={rejectRequest.error}
        onConfirm={(reason, discard) => decision.reject(proposal.number, reason, discard)}
      />
      <StopAskingDialog
        open={stopAsking}
        onOpenChange={setStopAsking}
        watch={watch}
        out={out}
        codeUnread={codeUnread}
        codeError={preview.error}
        pending={approving.isPending}
        error={approving.error}
        onConfirm={() => decision.approve(proposal.number, true, () => setStopAsking(false))}
      />
    </section>
  );
}

const NO_PATHS: string[] = [];

function loadOf(file: FileLoad): MissingLoad | null {
  if (file.isFetching) return "loading";
  if (file.error) return "failed";
  if (file.data?.truncated) return "too-large";
  return file.data ? null : "loading";
}

function useShownCode(
  watchId: number,
  proposal: Proposal,
  code: ProposalDetail | undefined,
  commit: string | null,
): ShownCode {
  const query = useProposal(commit ? watchId : null, commit ? proposal.number : null, proposal, commit ?? undefined);
  const source = commit ? query.data : code;
  const scope = `${proposal.workSha}:${commit ?? ""}`;
  const [asked, setAsked] = useState({ scope, paths: NO_PATHS });
  const paths = asked.scope === scope ? asked.paths : NO_PATHS;
  const files = useProposalFiles(watchId, proposal.number, proposal, commit ?? undefined, paths);
  const loaded = useMemo(
    () => files.flatMap((f) => (f.data && !f.data.truncated ? readPatch(f.data.diff, f.data.files ?? []) : [])),
    [files],
  );
  const diff = useMemo(
    () => (source ? readProposalDiff(source.diff, source.files ?? [], loaded) : null),
    [source, loaded],
  );
  const loads = useMemo(
    () =>
      new Map(
        paths.flatMap((path, i) => {
          const load = loadOf(files[i]);
          return load ? [[path, load] as const] : [];
        }),
      ),
    [paths, files],
  );
  const load = (path: string) => {
    const i = paths.indexOf(path);
    if (i >= 0) void files[i].refetch();
    else setAsked({ scope, paths: [...paths, path] });
  };
  return {
    diff,
    base: commit ? source?.base : proposal.headSha,
    truncated: source?.truncated ?? false,
    loading: query.isFetching,
    error: commit ? (query.error?.message ?? source?.codeError ?? null) : null,
    reload: () => void query.refetch(),
    loads,
    load,
  };
}

function SplitButton({
  variant,
  label,
  more,
  children,
}: {
  variant: "default" | "outline";
  label: string;
  more: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <ButtonGroup>
      {children}
      {more ? (
        <>
          {variant === "default" ? <ButtonGroupSeparator className="bg-primary-foreground/30" /> : null}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="icon-sm" variant={variant} className="w-7" aria-label={label}>
                <ChevronDownIcon />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">{more}</DropdownMenuContent>
          </DropdownMenu>
        </>
      ) : null}
    </ButtonGroup>
  );
}

function DecisionButtons({
  hasPush,
  pushes,
  approving,
  codeUnread,
  onApprove,
  onStopAsking,
  onReject,
  onRejectPush,
}: {
  hasPush: boolean;
  pushes: boolean;
  approving: boolean;
  codeUnread: boolean;
  onApprove: () => void;
  onStopAsking: () => void;
  onReject: () => void;
  onRejectPush: () => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <SplitButton
        variant="default"
        label="More ways to approve"
        more={
          <DropdownMenuItem disabled={codeUnread} onSelect={onStopAsking}>
            Approve and stop asking
          </DropdownMenuItem>
        }
      >
        <Button size="sm" disabled={approving || codeUnread} onClick={onApprove}>
          {approving ? <Spinner data-icon="inline-start" /> : <CheckIcon data-icon="inline-start" />}
          {hasPush ? "Approve" : "Approve and post"}
        </Button>
      </SplitButton>
      <SplitButton
        variant="outline"
        label="More ways to reject"
        more={
          pushes ? (
            <DropdownMenuItem variant="destructive" onSelect={onRejectPush}>
              Reject push
            </DropdownMenuItem>
          ) : null
        }
      >
        <Button size="sm" variant="outline" onClick={onReject}>
          Reject
        </Button>
      </SplitButton>
    </div>
  );
}

function noteLine(out: Outgoing, p: Proposal): string {
  const replies = `posts ${count(out.replies, "reply", "replies")}`;
  if (!p.hasPush) return replies;
  if (!out.pushes) return `${replies} · pushes nothing`;
  return `pushes ${commitsWord(out.commits, "commits not read yet")} to ${out.headRef} · lease pinned to ${shortSha(out.head)} · ${replies}`;
}

function replyWho(r: ProposalReply): { who: string; label: string } {
  const actor = r.answers?.actor;
  if (actor) return { who: actor, label: `Reply to ${actor}` };
  return { who: "the pull request", label: "Comment on the pull request" };
}

function whereOf(r: ProposalReply): string | null {
  const path = r.answers?.payload?.path;
  if (typeof path !== "string" || !path) return null;
  const line = r.answers?.payload?.line;
  return typeof line === "number" ? `${path}:${line}` : path;
}

function ReplyCard({
  reply,
  note,
  value,
  edited,
  onChange,
  onDrop,
}: {
  reply: ProposalReply;
  note?: string;
  value: string;
  edited: boolean;
  onChange: (text: string) => void;
  onDrop: () => void;
}) {
  const { who, label } = replyWho(reply);
  const where = whereOf(reply);
  const comment = reply.answers?.payload?.body;
  const id = `reply-${reply.id}`;
  return (
    <div className="flex flex-col gap-1.5 border-t pt-2.5 first-of-type:border-t-0 first-of-type:pt-0">
      <div className="flex items-center gap-2 text-sm">
        <MessageSquareReplyIcon className="size-4 shrink-0 text-muted-foreground" />
        {reply.answers ? (
          <span>
            <span className="font-medium">{who}</span> commented{" "}
            {where ? (
              <>
                on <code className="font-mono text-xs/normal">{where}</code>
              </>
            ) : (
              "on the conversation"
            )}
          </span>
        ) : (
          <span>On the conversation</span>
        )}
        {reply.answers ? <Meta>{relativeTime(reply.answers.at)}</Meta> : null}
        {edited ? (
          <Badge variant="outline" className="font-mono">
            edited
          </Badge>
        ) : null}
        <Button variant="ghost" size="xs" className="ml-auto" onClick={onDrop}>
          <Trash2Icon data-icon="inline-start" />
          Drop reply
        </Button>
      </div>
      {note ? <p className="text-xs text-attention">{note}</p> : null}
      {typeof comment === "string" && comment ? <p className="text-sm text-foreground/75">{comment}</p> : null}
      <label htmlFor={id} className="sr-only">
        {label}
      </label>
      <Textarea id={id} rows={2} value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}

function CodeUnread({ preview }: { preview: Preview }) {
  return (
    <Alert variant="destructive">
      <CircleXIcon />
      <AlertTitle>The code of this proposal could not be read, so the commits and files are unknown.</AlertTitle>
      <AlertDescription className="flex flex-wrap items-center gap-2">
        <span>{preview.error}</span>
        <Button size="xs" variant="outline" disabled={preview.loading} onClick={preview.reload}>
          {preview.loading ? <Spinner data-icon="inline-start" /> : <RotateCcwIcon data-icon="inline-start" />}
          Read the code again
        </Button>
      </AlertDescription>
    </Alert>
  );
}

function ProposalCode({ preview, ...area }: { preview: Preview } & ComponentProps<typeof CodeArea>) {
  if (preview.error) return <CodeUnread preview={preview} />;
  if (!preview.ready || !preview.detail) return <Spinner />;
  return <CodeArea {...area} />;
}

function FailedSection({ watch, proposal }: { watch: Watch; proposal: Proposal }) {
  const decision = useProposalDecision();
  const { retrying: retry, rejecting: reject } = decision;
  const [rejecting, setRejecting] = useState(false);
  const push = (proposal.error ?? "").startsWith("push");
  return (
    <section aria-label="Proposal" className="flex flex-col gap-3 border-b px-5 py-3.5">
      <Heading proposal={proposal}>
        <ToneBadge tone="bad">{push ? "push failed" : "release failed"}</ToneBadge>
        {proposal.approvedAt ? <Meta>approved {relativeTime(proposal.approvedAt)}</Meta> : null}
        <Meta>head {shortSha(proposal.headSha)}</Meta>
        {proposal.hasPush ? <Meta>work {shortSha(proposal.workSha)}</Meta> : null}
      </Heading>
      <Alert variant="destructive">
        <TriangleAlertIcon />
        <AlertTitle>
          {push ? "The push did not go out, so nothing was posted either." : "The replies did not all go out."}
        </AlertTitle>
        <AlertDescription>{failure(proposal)}. Retry once the cause is gone.</AlertDescription>
      </Alert>
      {retry.error ? <p className="text-sm text-destructive">{retry.error.message}</p> : null}
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" disabled={retry.isPending} onClick={() => decision.retry(proposal.number)}>
          {retry.isPending ? <Spinner data-icon="inline-start" /> : <RotateCcwIcon data-icon="inline-start" />}
          {push ? "Retry the push" : "Retry"}
        </Button>
        <Button size="sm" variant="outline" onClick={() => setRejecting(true)}>
          Reject proposal
        </Button>
        {push ? <Meta className="ml-auto">retries with the lease pinned to {shortSha(proposal.headSha)}</Meta> : null}
      </div>
      <RejectProposalDialog
        open={rejecting}
        onOpenChange={setRejecting}
        watch={watch}
        number={proposal.number}
        head={proposal.headSha}
        pending={reject.isPending}
        error={reject.error}
        onConfirm={(reason, discard) => decision.reject(proposal.number, reason, discard)}
      />
    </section>
  );
}

function ConflictSection({ watch, proposal }: { watch: Watch; proposal: Proposal }) {
  const after =
    watch.approvalMode === "manual"
      ? "Its resolution comes back as a new proposal, and that one asks you even with a clean rebase or merge set to approve on its own, because nobody has read it."
      : "The watch runs in auto, so the daemon pushes its resolution when the turn of the agent ends.";
  return (
    <section aria-label="Proposal" className="flex flex-col gap-3 border-b px-5 py-3.5">
      <Heading proposal={proposal}>
        <ToneBadge tone="bad">rebase conflicts</ToneBadge>
        <Meta>head {shortSha(proposal.headSha)}</Meta>
      </Heading>
      <p className="text-sm text-foreground/80">
        The pull request branch moved under proposal {proposal.number}, and {failure(proposal)}. The daemon aborted the
        rebase and handed it to the agent. {after}
      </p>
    </section>
  );
}
