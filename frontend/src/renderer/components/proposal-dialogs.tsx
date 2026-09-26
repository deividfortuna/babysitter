import { useState, type ReactNode } from "react";
import { CircleAlertIcon, TriangleAlertIcon } from "lucide-react";
import type { Watch } from "@/hooks/useWatches";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import { shortSha } from "@/lib/time";
import { watchLabel } from "@/lib/watch-status";

export function count(n: number, one: string, many = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`;
}

export function joinAnd(words: string[]): string {
  if (words.length < 2) return words.join("");
  return `${words.slice(0, -1).join(", ")} and ${words.at(-1)}`;
}

export type Outgoing = {
  commits: number | null;
  replies: number;
  edited: number;
  headRef: string;
  head: string;
  pushes: boolean;
};

export function commitsWord(commits: number | null, unknown = "commits"): string {
  return commits === null ? unknown : count(commits, "commit");
}

function WhatGoesOut({ out }: { out: Outgoing }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="eyebrow">What goes out now</span>
      <ul className="flex list-disc flex-col gap-0.5 pl-5 text-sm text-foreground/85">
        {out.pushes && out.commits !== 0 ? (
          <li>
            {commitsWord(out.commits)} pushed to <code className="font-mono text-xs/normal">{out.headRef}</code> with
            the lease pinned to <code className="font-mono text-xs/normal">{shortSha(out.head)}</code>
          </li>
        ) : null}
        {out.replies > 0 ? (
          <li>
            {count(out.replies, "reply", "replies")} posted under your account
            {out.edited > 0 ? `, ${out.edited} as you edited ${out.edited === 1 ? "it" : "them"}` : ""}
          </li>
        ) : null}
      </ul>
    </div>
  );
}

function ProposalDialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  action,
  destructive,
  pending,
  disabled,
  error,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: ReactNode;
  children?: ReactNode;
  action: string;
  destructive?: boolean;
  pending?: boolean;
  disabled?: boolean;
  error?: Error | null;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-(--size-dialog) sm:max-w-(--size-dialog-max)">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {children}
        {error ? (
          <Alert variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>{error.message}</AlertTitle>
          </Alert>
        ) : null}
        <DialogFooter className="sm:justify-start">
          <Button
            type="button"
            variant={destructive ? "destructive" : "default"}
            disabled={pending || disabled}
            onClick={onConfirm}
          >
            {pending ? <Spinner data-icon="inline-start" /> : null}
            {action}
          </Button>
          <DialogClose asChild>
            <Button type="button" variant="ghost">
              Cancel
            </Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type Controlled = { open: boolean; onOpenChange: (open: boolean) => void };

export function DropReplyDialog({
  open,
  onOpenChange,
  who,
  answers,
  text,
  onConfirm,
}: Controlled & { who: string; answers: boolean; text: string; onConfirm: () => void }) {
  const description = answers
    ? `The reply to ${who} is not posted. Their comment was never marked seen, so it reaches the agent again on the next poll, with word that you dropped this answer.`
    : "The comment on the pull request is not posted. It answers no comment, so nothing comes back to the agent.";
  return (
    <ProposalDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Drop this reply?"
      description={description}
      action="Drop reply"
      destructive
      onConfirm={onConfirm}
    >
      <pre className="max-h-40 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-2xs/relaxed whitespace-pre-wrap">
        {text}
      </pre>
    </ProposalDialog>
  );
}

export function RejectPushDialog({
  open,
  onOpenChange,
  number,
  commits,
  replies,
  headRef,
  onConfirm,
}: Controlled & { number: number; commits: string[] | null; replies: number; headRef: string; onConfirm: () => void }) {
  return (
    <ProposalDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Reject the push of proposal ${number}?`}
      description={
        <>
          The {commits ? count(commits.length, "commit stays", "commits stay") : "commits stay"} on the work branch and
          nothing is pushed to <code className="font-mono text-xs/normal">{headRef}</code>. The rest of the proposal
          still waits for you.
        </>
      }
      action="Reject push"
      destructive
      onConfirm={onConfirm}
    >
      {replies > 0 ? <RepliesStillGoOut replies={replies} commits={commits ?? []} /> : null}
    </ProposalDialog>
  );
}

function RepliesStillGoOut({ replies, commits }: { replies: number; commits: string[] }) {
  const one = replies === 1;
  const named =
    commits.length > 0
      ? `${one ? "It" : "They"} may name ${joinAnd(commits.map(shortSha))}, which the pull request will not have. `
      : "";
  return (
    <Alert>
      <TriangleAlertIcon />
      <AlertTitle>
        {one
          ? "The reply still goes out, and it describes this code."
          : "The replies still go out, and they describe this code."}
      </AlertTitle>
      <AlertDescription>
        {named}
        {one ? "Edit or drop it before you approve." : "Edit or drop them before you approve."}
      </AlertDescription>
    </Alert>
  );
}

export function RejectProposalDialog({
  open,
  onOpenChange,
  watch,
  number,
  head,
  pending,
  error,
  onConfirm,
}: Controlled & {
  watch: Watch;
  number: number;
  head: string;
  pending: boolean;
  error: Error | null;
  onConfirm: (reason: string, discard: boolean) => void;
}) {
  const [reason, setReason] = useState("");
  const [discard, setDiscard] = useState(false);
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setReason("");
      setDiscard(false);
    }
  }

  return (
    <ProposalDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Reject proposal ${number} of ${watchLabel(watch)}`}
      description="Nothing is pushed and nothing is posted. The agent gets your reason and can answer with a new proposal."
      action="Reject proposal"
      destructive
      pending={pending}
      error={error}
      onConfirm={() => onConfirm(reason.trim(), discard)}
    >
      <Field>
        <FieldLabel htmlFor="reject-reason">Reason (optional)</FieldLabel>
        <Textarea
          id="reject-reason"
          rows={3}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="What the agent should do instead. It reads this with everything it was not told yet."
        />
      </Field>
      <Field orientation="horizontal">
        <Checkbox id="reject-discard" checked={discard} onCheckedChange={(v) => setDiscard(v === true)} />
        <FieldLabel htmlFor="reject-discard" className="font-normal">
          Discard the commits
        </FieldLabel>
      </Field>
      <FieldDescription>
        The work branch goes back to <code className="font-mono text-xs/normal">{shortSha(head)}</code>, whatever the
        setting of the worktree says. Left off, the commits stay on the work branch and the agent can build on them.
      </FieldDescription>
    </ProposalDialog>
  );
}

export function StopAskingDialog({
  open,
  onOpenChange,
  watch,
  out,
  codeUnread,
  codeError,
  pending,
  error,
  onConfirm,
}: Controlled & {
  watch: Watch;
  out: Outgoing;
  codeUnread: boolean;
  codeError: string | null;
  pending: boolean;
  error: Error | null;
  onConfirm: () => void;
}) {
  return (
    <ProposalDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Approve and stop asking on ${watchLabel(watch)}`}
      description="This proposal goes out now, and the watch runs in auto from here: the daemon pushes and posts as soon as each turn ends. Your other watches still ask."
      action="Approve and switch to auto"
      pending={pending}
      disabled={codeUnread}
      error={error}
      onConfirm={onConfirm}
    >
      <WhatGoesOut out={out} />
      {codeUnread ? <CodeNotRead error={codeError} /> : null}
    </ProposalDialog>
  );
}

function CodeNotRead({ error }: { error: string | null }) {
  return (
    <Alert variant="destructive">
      <CircleAlertIcon />
      <AlertTitle>The code of this proposal is not read, so it cannot be released yet.</AlertTitle>
      {error ? <AlertDescription>{error}</AlertDescription> : null}
    </Alert>
  );
}

export function SwitchToAutoDialog({
  open,
  onOpenChange,
  watch,
  number,
  out,
  codeUnread,
  codeError,
  pending,
  error,
  onConfirm,
}: Controlled & {
  watch: Watch;
  number: number;
  out: Outgoing | null;
  codeUnread: boolean;
  codeError: string | null;
  pending: boolean;
  error: Error | null;
  onConfirm: () => void;
}) {
  return (
    <ProposalDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Switch ${watchLabel(watch)} to auto`}
      description={`Proposal ${number} waits on you, and auto means nothing waits. Switching releases it now, so read it first if you have not.`}
      action="Release and switch to auto"
      pending={pending}
      disabled={codeUnread}
      error={error}
      onConfirm={onConfirm}
    >
      {out ? <WhatGoesOut out={out} /> : null}
      {codeUnread ? <CodeNotRead error={codeError} /> : null}
    </ProposalDialog>
  );
}
