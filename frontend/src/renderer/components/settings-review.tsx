import { MergeMethodSelect } from "@/components/merge-method-select";
import { OptionSelect } from "@/components/option-select";
import {
  DaemonSettings,
  DraftNumberRow,
  SettingsCard,
  SettingsRow,
  SettingsSection,
  useTrackedWrite,
} from "@/components/settings-page";
import { Switch } from "@/components/ui/switch";
import { useDraftField } from "@/hooks/use-draft-field";
import type { Settings } from "@/hooks/useSettings";
import { approvalsField, approvalsInvalid, approvalsRequired } from "@/lib/approvals";
import { BRANCH_UPDATES } from "@/lib/branch-update";

function parseApprovals(text: string): number | null | undefined {
  if (approvalsInvalid(text)) return undefined;
  return approvalsRequired(text) ?? null;
}

export function ReviewPanel() {
  return <DaemonSettings>{(settings) => <ReviewForm settings={settings} />}</DaemonSettings>;
}

function ReviewForm({ settings }: { settings: Settings }) {
  const save = useTrackedWrite();
  const approvals = useDraftField<number | null>({
    value: settings.approvalsRequired ?? null,
    format: approvalsField,
    parse: parseApprovals,
    commit: (approvalsRequired) => save({ approvalsRequired }),
  });

  return (
    <div className="flex flex-col gap-4.5">
      <SettingsSection label="Ready to merge">
        <SettingsCard>
          <DraftNumberRow
            id="approvals"
            label="Approvals before ready to merge"
            description="Empty takes the rule of the base branch. Zero asks for no review."
            error="The approvals take a whole number, 0 or more."
            min={0}
            field={approvals}
          />
          <SettingsRow
            label="Merge method"
            htmlFor="merge-method"
            description="Empty takes the first method the repository allows."
          >
            <MergeMethodSelect
              id="merge-method"
              className="w-44"
              value={settings.mergeMethod}
              onChange={(mergeMethod) => save({ mergeMethod })}
            />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection label="Branch behind its base">
        <SettingsCard>
          <SettingsRow
            label="Branch update"
            htmlFor="branch-update"
            description="Rebase rewrites the branch on top of its base. Merge adds a merge commit and keeps the history. The agent solves a conflict the same way."
          >
            <OptionSelect
              id="branch-update"
              label="Branch behind its base"
              size="default"
              className="w-44"
              options={BRANCH_UPDATES}
              value={settings.branchUpdate}
              onChange={(branchUpdate) => save({ branchUpdate })}
            />
          </SettingsRow>
          <SettingsRow
            label="Update the branch on GitHub first"
            htmlFor="update-on-github"
            description="GitHub updates the branch with no turn of the agent. The agent does it only when GitHub refuses."
          >
            <Switch
              id="update-on-github"
              checked={settings.updateOnGitHub}
              onCheckedChange={(on) => save({ updateOnGitHub: on })}
            />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection label="The first poll">
        <SettingsCard>
          <SettingsRow
            label="Report the review items that already exist"
            htmlFor="include-existing"
            description="The first poll hands the agent what is on the pull request already."
          >
            <Switch
              id="include-existing"
              checked={settings.includeExisting}
              onCheckedChange={(on) => save({ includeExisting: on })}
            />
          </SettingsRow>
          <SettingsRow
            label="Report my own comments"
            htmlFor="include-own"
            description="For a repository where you review your own work."
          >
            <Switch id="include-own" checked={settings.includeOwn} onCheckedChange={(on) => save({ includeOwn: on })} />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    </div>
  );
}
