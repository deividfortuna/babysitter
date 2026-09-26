import { useMemo, useState } from "react";
import {
  CheckIcon,
  CircleXIcon,
  FileDiffIcon,
  MessageSquareReplyIcon,
  RotateCcwIcon,
  Trash2Icon,
  TriangleAlertIcon,
  Undo2Icon,
} from "lucide-react";
import type { Proposal, ProposalReply } from "@/hooks/useProposals";
import type { Watch } from "@/hooks/useWatches";
import { AttentionBadge, Meta, ToneBadge } from "@/components/status-badges";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { relativeTime, shortSha } from "@/lib/time";
import { cn } from "@/lib/utils";

function conflicted(p: Proposal): boolean {
  return p.status === "failed" && (p.error ?? "").includes(" conflicts in ");
}

function failure(p: Proposal): string {
  return (p.error ?? "").replace(/^[^:]*proposal \d+: /, "");
}

function splitDiff(diff: string): Map<string, string> {
  const files = new Map<string, string>();
  for (const part of diff.split(/^(?=diff --git )/m)) {
    const header = part.split("\n", 1)[0];
    const path = header.split(" b/").at(-1) ?? "";
    if (path) files.set(path, part);
  }
  return files;
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

  const replies = detail?.replies ?? proposal.replies ?? [];
  const bot = watch.dependabot;
  const pushes = proposal.hasPush && !pushRejected;
  const out = decision.out ?? outgoing(watch, proposal, detail, draft);

  const intro = bot
    ? "The agent answered a comment. Dependabot owns this branch, so the proposal carries replies only: the daemon pushes nothing and rebases nothing."
    : proposal.rebasedFrom
      ? "The pull request branch moved while this waited, and the daemon rebased the work onto it without conflicts. It is the change you were reading, on new commits, and a reply that named an old commit now names the new one."
      : "The agent finished a turn. None of it is on GitHub yet: what you approve is what goes out, under your account.";

  return (
    <section aria-label="Proposal" className="flex flex-col gap-3 border-b px-5 py-3.5">
      <Heading proposal={proposal}>
        <AttentionBadge>approval needed</AttentionBadge>
        {proposal.rebasedFrom ? (
          <Badge variant="outline" className="font-mono">
            rebased
          </Badge>
        ) : null}
        <Meta>
          {proposal.rebasedFrom ? "rebased" : "opened"}{" "}
          {relativeTime((proposal.rebasedFrom ? proposal.endedAt : proposal.openedAt) ?? proposal.openedAt)}
        </Meta>
        <Meta>head {shortSha(proposal.headSha)}</Meta>
        {proposal.hasPush ? (
          <Meta>
            work {shortSha(proposal.workSha)}
            {proposal.rebasedFrom ? `, was ${shortSha(proposal.rebasedFrom)}` : ""}
          </Meta>
        ) : null}
      </Heading>
      <p className="text-sm text-foreground/80">{intro}</p>

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

      {proposal.hasPush ? <CodeArea preview={preview} dimmed={pushRejected} /> : null}

      {replies.length > 0 ? (
        <div className="flex flex-col gap-2.5">
          <span className="eyebrow">
            {count(replies.length - dropped.length, "reply", "replies")} · posted under your account
            {dropped.length > 0 ? ` · ${dropped.length} dropped` : ""}
          </span>
          {replies.map((r) =>
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
                value={edits[r.id] ?? r.edited ?? r.body}
                edited={edits[r.id] !== undefined && edits[r.id] !== r.body}
                onChange={(text) => change((d) => ({ ...d, edits: { ...d.edits, [r.id]: text } }))}
                onDrop={() => setDropping(r)}
              />
            ),
          )}
        </div>
      ) : null}

      {approving.error || rejectRequest.error ? (
        <Alert variant="destructive">
          <CircleXIcon />
          <AlertTitle>{(approving.error ?? rejectRequest.error)?.message}</AlertTitle>
        </Alert>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          disabled={approving.isPending || codeUnread}
          onClick={() => decision.approve(proposal.number, false)}
        >
          {approving.isPending ? <Spinner data-icon="inline-start" /> : <CheckIcon data-icon="inline-start" />}
          {proposal.hasPush ? "Approve" : "Approve and post"}
        </Button>
        <Button size="sm" variant="outline" disabled={codeUnread} onClick={() => setStopAsking(true)}>
          Approve and stop asking
        </Button>
        <Button size="sm" variant="outline" onClick={() => setRejecting(true)}>
          Reject proposal
        </Button>
        {pushes ? (
          <Button size="sm" variant="ghost" className="text-destructive" onClick={() => setRejectingPush(true)}>
            Reject push
          </Button>
        ) : null}
        <Meta className="ml-auto">{noteLine(out, proposal)}</Meta>
      </div>

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
  value,
  edited,
  onChange,
  onDrop,
}: {
  reply: ProposalReply;
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

function CodeArea({ preview, dimmed }: { preview: Preview; dimmed: boolean }) {
  const detail = preview.ready ? preview.detail : undefined;
  const files = detail?.files ?? [];
  const commits = detail?.commits ?? [];
  const heldBack = commits.filter((c) => c.heldBack).length;
  const diffs = useMemo(() => splitDiff(detail?.diff ?? ""), [detail?.diff]);
  const [picked, setPicked] = useState<string | null>(null);
  const selected = picked ?? files[0]?.path ?? null;
  if (preview.error) return <CodeUnread preview={preview} />;
  if (!detail) return <Spinner />;
  return (
    <div className={cn("grid gap-4 md:grid-cols-[300px_1fr]", dimmed && "opacity-50")}>
      <div className="flex min-w-0 flex-col gap-3">
        <div className="flex flex-col gap-1">
          <span className="eyebrow">{count(commits.length, "commit")} on the work branch</span>
          {commits.map((c) => (
            <div key={c.sha} className="flex items-baseline gap-2 text-sm">
              <Meta>{shortSha(c.sha)}</Meta>
              <span className="truncate">{c.subject}</span>
              {c.heldBack ? (
                <Badge variant="outline" className="shrink-0 font-mono">
                  kept off before
                </Badge>
              ) : null}
            </div>
          ))}
          {heldBack > 0 ? (
            <p className="text-sm text-attention">
              {heldBack === 1
                ? "1 commit here is one you kept off the pull request in an earlier decision. Approving pushes it with the rest."
                : `${heldBack} commits here are ones you kept off the pull request in an earlier decision. Approving pushes them with the rest.`}
            </p>
          ) : null}
        </div>
        <div className="flex flex-col gap-0.5">
          <span className="eyebrow">{count(files.length, "changed file")}</span>
          {files.map((f) => (
            <button
              key={f.path}
              type="button"
              aria-pressed={f.path === selected}
              onClick={() => setPicked(f.path)}
              className={cn(
                "flex items-baseline justify-between gap-2 rounded-md px-2 py-1 text-left hover:bg-muted",
                f.path === selected && "bg-accent",
              )}
            >
              <span className="truncate font-mono text-xs/normal">{f.path}</span>
              <Meta className="shrink-0">
                +{f.added} −{f.deleted}
              </Meta>
            </button>
          ))}
        </div>
      </div>
      <pre
        aria-label="Diff"
        className="h-90 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-2xs/relaxed whitespace-pre"
      >
        {(selected ? diffs.get(selected) : undefined) ?? detail.diff}
        {detail.truncated ? "\n… the diff is longer than one megabyte and was cut here" : ""}
      </pre>
    </div>
  );
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
      ? "Its resolution comes back as a new proposal, and that one asks you even with a clean rebase set to approve on its own, because nobody has read it."
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
