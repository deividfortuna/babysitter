import { useState } from "react";
import { CircleAlertIcon, PanelRightDashedIcon } from "lucide-react";
import { useSetApproval, type ApprovalMode } from "@/hooks/useProposals";
import { useUpdateWatch, type Watch } from "@/hooks/useWatches";
import { useProposalDecision } from "@/components/proposal-decision";
import { SwitchToAutoDialog } from "@/components/proposal-dialogs";
import { ApprovalModeSelect } from "@/components/approval-mode-select";
import { BranchUpdateSelect } from "@/components/branch-update-select";
import { MergeMethodSelect } from "@/components/merge-method-select";
import { SettingRow } from "@/components/setting-row";
import { ToneBadge } from "@/components/status-badges";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { approvalsField, approvalsInvalid, approvalsRequired } from "@/lib/approvals";
import { isSelfWatch, watchLabel } from "@/lib/watch-status";

type Props = { watch: Watch; onClose: () => void };

export function WatchSettingsPanel({ watch, onClose }: Props) {
  const setApproval = useSetApproval();
  const approvalsRules = useUpdateWatch();
  const methodRules = useUpdateWatch();
  const readyRules = useUpdateWatch();
  const branchRules = useUpdateWatch();
  const failure =
    setApproval.error ?? approvalsRules.error ?? methodRules.error ?? readyRules.error ?? branchRules.error;

  return (
    <aside
      aria-labelledby="watch-settings-title"
      className="relative flex w-90 shrink-0 flex-col gap-5 overflow-y-auto border-l bg-background p-5"
    >
      <Button
        variant="ghost"
        size="icon"
        className="absolute top-2.5 right-5 size-7"
        aria-label="Close watch settings"
        title="Close watch settings"
        onClick={onClose}
      >
        <PanelRightDashedIcon />
      </Button>
      <div className="flex flex-col gap-1.5 pr-6">
        <h2 id="watch-settings-title" className="text-base font-semibold">
          Watch settings
        </h2>
        <p className="text-sm text-muted-foreground">For {watchLabel(watch)} only. They start as your defaults.</p>
      </div>

      {isSelfWatch(watch) ? (
        <SettingRow
          label="Approval mode"
          description="Your own coding session pushes and replies itself, so this watch has no gate."
        >
          <span title="A watch your own coding session drives has no gate">
            <ToneBadge tone="neutral">auto</ToneBadge>
          </span>
        </SettingRow>
      ) : (
        <ApprovalRows watch={watch} setApproval={setApproval} />
      )}

      <ApprovalsRow
        key={watch.approvalsRequired}
        watch={watch}
        pending={approvalsRules.isPending}
        onChange={(approvalsRequired, done) =>
          approvalsRules.mutate({ id: watch.id, approvalsRequired }, { onSuccess: (w) => done(w.approvalsRequired) })
        }
      />

      <SettingRow label="Merge method" htmlFor="watch-merge-method" description="Used by Merge in the header.">
        <MergeMethodSelect
          id="watch-merge-method"
          size="sm"
          className="w-32 shrink-0"
          value={watch.mergeMethod ?? ""}
          disabled={methodRules.isPending}
          onChange={(mergeMethod) => methodRules.mutate({ id: watch.id, mergeMethod })}
        />
      </SettingRow>

      <SettingRow label="Merge when ready" htmlFor="watch-merge-when-ready" description={mergeWhenReadyText(watch)}>
        <Switch
          id="watch-merge-when-ready"
          checked={watch.mergeWhenReady}
          disabled={readyRules.isPending}
          onCheckedChange={(mergeWhenReady) => readyRules.mutate({ id: watch.id, mergeWhenReady })}
        />
      </SettingRow>

      <SettingRow
        label="Branch behind its base"
        htmlFor="watch-branch-update"
        description={
          watch.dependabot
            ? "Dependabot owns the branch, so only the bot updates it."
            : "The agent solves a conflict the same way."
        }
      >
        <BranchUpdateSelect
          id="watch-branch-update"
          size="sm"
          className="w-32 shrink-0"
          value={watch.branchUpdate}
          disabled={branchRules.isPending || watch.dependabot}
          onChange={(branchUpdate) => branchRules.mutate({ id: watch.id, branchUpdate })}
        />
      </SettingRow>

      <SettingRow
        label="Update the branch on GitHub first"
        htmlFor="watch-update-on-github"
        description={updateOnGitHubText(watch)}
      >
        <Switch
          id="watch-update-on-github"
          checked={watch.updateOnGitHub}
          disabled={branchRules.isPending || !githubUpdates(watch)}
          onCheckedChange={(updateOnGitHub) => branchRules.mutate({ id: watch.id, updateOnGitHub })}
        />
      </SettingRow>

      {failure ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{failure.message}</AlertTitle>
        </Alert>
      ) : null}
    </aside>
  );
}

function githubUpdates(watch: Watch): boolean {
  const branchOfSomeoneElse = watch.dependabot || isSelfWatch(watch);
  return !branchOfSomeoneElse;
}

function updateOnGitHubText(watch: Watch): string {
  if (watch.dependabot) return "Dependabot owns the branch, so only the bot updates it.";
  if (isSelfWatch(watch)) return "Your own coding session pushes the branch, so it updates the branch itself.";
  return "The agent does it only when GitHub refuses.";
}

function mergeWhenReadyText(watch: Watch): string {
  const always = "The daemon merges as soon as the watch is ready to merge.";
  const onByScope = watch.autoReason === "dependabot" && watch.mergeWhenReady && Boolean(watch.updateType);
  if (!onByScope) return always;
  return `${always} On because ${watch.updateType} is within the scope of the repository.`;
}

function ApprovalRows({ watch, setApproval }: { watch: Watch; setApproval: ReturnType<typeof useSetApproval> }) {
  const decision = useProposalDecision();
  const [switching, setSwitching] = useState(false);
  const pending = watch.pendingProposal ?? null;
  const read = decision.current?.number === pending;
  const auto = watch.approvalMode === "auto";

  function change(mode: ApprovalMode) {
    if (mode === "auto" && pending) {
      decision.approving.reset();
      setSwitching(true);
      return;
    }
    setApproval.mutate({ id: watch.id, mode });
  }

  return (
    <>
      <SettingRow
        label="Approval mode"
        htmlFor="watch-approval-mode"
        description="Manual holds each turn until you approve it. Switching to auto releases a pending proposal."
      >
        <ApprovalModeSelect
          id="watch-approval-mode"
          size="sm"
          className="w-32 shrink-0"
          value={watch.approvalMode}
          disabled={setApproval.isPending}
          onChange={change}
        />
      </SettingRow>
      <SettingRow
        label="Approve a clean rebase on its own"
        htmlFor="watch-auto-rebase"
        description="Approved work does not ask again because the branch moved. No effect in auto."
      >
        <Switch
          id="watch-auto-rebase"
          checked={watch.autoApproveRebase}
          disabled={auto || setApproval.isPending}
          onCheckedChange={(on) => setApproval.mutate({ id: watch.id, autoApproveRebase: on })}
        />
      </SettingRow>
      <SwitchToAutoDialog
        open={switching}
        onOpenChange={setSwitching}
        watch={watch}
        number={pending ?? 0}
        out={read ? (decision.out ?? null) : null}
        codeUnread={!read || decision.codeUnread}
        codeError={decision.preview.error}
        pending={decision.approving.isPending}
        error={decision.approving.error}
        onConfirm={() => pending && decision.approve(pending, true, () => setSwitching(false))}
      />
    </>
  );
}

function ApprovalsRow({
  watch,
  pending,
  onChange,
}: {
  watch: Watch;
  pending: boolean;
  onChange: (approvals: number | null, done: (saved: number) => void) => void;
}) {
  const [draft, setDraft] = useState(approvalsField(watch.approvalsRequired));
  const [invalid, setInvalid] = useState(false);

  function commit() {
    if (pending) return;
    const refused = approvalsInvalid(draft);
    setInvalid(refused);
    if (refused) return;
    const wanted = approvalsRequired(draft) ?? null;
    if (wanted === watch.approvalsRequired) return;
    onChange(wanted, (saved) => setDraft(approvalsField(saved)));
  }

  return (
    <SettingRow
      label="Approvals before ready to merge"
      htmlFor="watch-approvals"
      description={
        invalid ? (
          <span className="text-destructive">Use a whole number from 0, or leave it empty.</span>
        ) : (
          "Empty takes the rule of the base branch."
        )
      }
    >
      <Input
        id="watch-approvals"
        type="number"
        inputMode="numeric"
        min={0}
        className="h-8 w-18 shrink-0"
        value={draft}
        aria-invalid={invalid || undefined}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter") commit();
        }}
      />
    </SettingRow>
  );
}
