import { useState, type ReactNode } from "react";
import { ChevronDownIcon, CircleAlertIcon, FolderOpenIcon, PanelRightDashedIcon } from "lucide-react";
import { useProviders, type Provider } from "@/hooks/useProviders";
import {
  useRepoConfig,
  useUpdateRepoConfig,
  type DependabotApproval,
  type DependabotScope,
  type Repo,
  type RepoConfig,
  type WatchOverrides,
} from "@/hooks/useRepos";
import { useSettings } from "@/hooks/useSettings";
import { AgentLogo } from "@/components/agent-logo";
import { mergeMethodLabel } from "@/components/merge-method-select";
import { OptionSelect, toOptions, type Option } from "@/components/option-select";
import { EffortSelect } from "@/components/effort-select";
import { SettingRow } from "@/components/setting-row";
import { Meta } from "@/components/status-badges";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { approvalsField, approvalsInvalid, approvalsRequired, wholeNumber } from "@/lib/approvals";
import {
  agentLabel,
  branchUpdateDefaultLabel,
  daemonDefaults,
  defaultLabel,
  effortDefaultLabel,
  effortsOf,
  mergeMethodDefaultLabel,
  modelLabel,
  overrideOf,
  repositoryDefaults,
  type WatchDefaults as Defaults,
} from "@/lib/watch-defaults";
import { BRANCH_UPDATES } from "@/lib/branch-update";
import { bridge } from "@/lib/bridge";
import { fromSelectValue, toSelectValue } from "@/lib/select-value";
import { shortDate } from "@/lib/time";
import { cn } from "@/lib/utils";

const NARROW_SELECT = "w-26 shrink-0 font-mono text-xs";
const OVERRIDE_SELECT = "w-40 shrink-0";

const SCOPES: Option<DependabotScope>[] = [
  { value: "patch", label: "patch" },
  { value: "minor", label: "minor" },
  { value: "major", label: "major" },
];

const APPROVALS: Option<DependabotApproval>[] = [
  { value: "never", label: "never" },
  { value: "ask", label: "ask" },
  { value: "green", label: "green" },
];

const APPROVAL_HELP: Record<DependabotApproval, string> = {
  never: "The daemon submits no review. A missing review waits for you.",
  ask: "A notification asks you to approve when the build is green and the update is in scope.",
  green: "Approve in your name when the build is green and the update is in scope.",
};

type Props = { repo: Repo; onClose: () => void };

export function RepoSettingsPanel({ repo, onClose }: Props) {
  const config = useRepoConfig(repo.id);

  return (
    <aside
      aria-labelledby="repo-settings-title"
      className="relative flex w-90 shrink-0 flex-col gap-5 overflow-y-auto border-l bg-background p-5"
    >
      <Button
        variant="ghost"
        size="icon"
        className="absolute top-2.5 right-5 size-7"
        aria-label="Close repository settings"
        title="Close repository settings"
        onClick={onClose}
      >
        <PanelRightDashedIcon />
      </Button>
      <div className="flex flex-col gap-1.5 pr-6">
        <h2 id="repo-settings-title" className="text-base font-semibold">
          Repository settings
        </h2>
        <p className="text-sm text-muted-foreground">
          What babysitter does with new pull requests of {repo.fullName}. Nothing starts until you turn it on.
        </p>
      </div>

      {config.error ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{config.error.message}</AlertTitle>
        </Alert>
      ) : null}
      {config.data ? <ConfigSections repo={repo} config={config.data} /> : null}
      {config.isPending ? (
        <div role="status" aria-label="Loading repository settings" className="flex flex-col gap-3">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      ) : null}
    </aside>
  );
}

function withSince(text: string, on: boolean, since: string | null | undefined): string {
  if (!on || !since) return text;
  return `${text} On since ${shortDate(since)}.`;
}

function ConfigSections({ repo, config }: { repo: Repo; config: RepoConfig }) {
  const toggles = useUpdateRepoConfig(repo.id);
  const dependabot = useUpdateRepoConfig(repo.id);
  const overrides = useUpdateRepoConfig(repo.id);
  const failure = toggles.error ?? dependabot.error ?? overrides.error;

  return (
    <>
      <CheckoutField key={config.checkoutDir} repo={repo} saved={config.checkoutDir} />

      <Section title="Auto start">
        <SettingRow
          label="My pull requests"
          htmlFor="repo-auto-start-mine"
          description={withSince(
            "Yours or assigned to you, opened from now on.",
            config.autoStartMine,
            config.autoStartMineSince,
          )}
        >
          <Switch
            id="repo-auto-start-mine"
            checked={config.autoStartMine}
            disabled={toggles.isPending}
            onCheckedChange={(autoStartMine) => toggles.mutate({ autoStartMine })}
          />
        </SettingRow>
        <SettingRow
          label="Include drafts"
          htmlFor="repo-include-drafts"
          description="A draft starts when it is marked ready."
        >
          <Switch
            id="repo-include-drafts"
            checked={config.includeDrafts}
            disabled={toggles.isPending}
            onCheckedChange={(includeDrafts) => toggles.mutate({ includeDrafts })}
          />
        </SettingRow>
        <SettingRow
          label="Dependabot"
          htmlFor="repo-auto-watch-dependabot"
          description={withSince(
            "Each new update gets a watch.",
            config.autoWatchDependabot,
            config.autoWatchDependabotSince,
          )}
        >
          <Switch
            id="repo-auto-watch-dependabot"
            checked={config.autoWatchDependabot}
            disabled={toggles.isPending}
            onCheckedChange={(autoWatchDependabot) => toggles.mutate({ autoWatchDependabot })}
          />
        </SettingRow>
      </Section>

      <Section title="Dependabot">
        <SettingRow
          label="Merge on its own up to"
          htmlFor="repo-dependabot-scope"
          description="Bigger updates stop at ready to merge."
        >
          <OptionSelect
            id="repo-dependabot-scope"
            className={NARROW_SELECT}
            options={SCOPES}
            value={config.dependabotScope}
            disabled={dependabot.isPending}
            onChange={(dependabotScope) => dependabot.mutate({ dependabotScope })}
          />
        </SettingRow>
        <SettingRow
          label="Approve for me"
          htmlFor="repo-dependabot-approval"
          description={APPROVAL_HELP[config.dependabotApproval]}
        >
          <OptionSelect
            id="repo-dependabot-approval"
            className={NARROW_SELECT}
            options={APPROVALS}
            value={config.dependabotApproval}
            disabled={dependabot.isPending}
            onChange={(dependabotApproval) => dependabot.mutate({ dependabotApproval })}
          />
        </SettingRow>
        <LimitRow
          key={config.dependabotLimit}
          limit={config.dependabotLimit}
          pending={dependabot.isPending}
          onChange={(dependabotLimit) => dependabot.mutate({ dependabotLimit })}
        />
      </Section>

      <WatchDefaults
        overrides={config.overrides}
        pending={overrides.isPending}
        onChange={(next) => overrides.mutate({ overrides: next })}
      />

      {failure ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{failure.message}</AlertTitle>
        </Alert>
      ) : null}
    </>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-4 border-t pt-4">
      <span className="eyebrow">{title}</span>
      {children}
    </div>
  );
}

function checkoutHelp(repo: Repo, saved: string): string {
  if (saved) return `Auto start makes each worktree from it. It must have a remote for ${repo.fullName}.`;
  return `Empty: babysitter clones ${repo.fullName} once into its data folder and makes each worktree from that clone.`;
}

function CheckoutField({ repo, saved }: { repo: Repo; saved: string }) {
  const update = useUpdateRepoConfig(repo.id);
  const [draft, setDraft] = useState(saved);
  const refusal = update.error?.message ?? null;

  function commit(dir: string) {
    const wanted = dir.trim();
    if (update.isPending || wanted === saved) return;
    update.mutate({ checkoutDir: wanted });
  }

  async function choose() {
    const dir = await bridge.dialog.pickDirectory(draft.trim() || undefined);
    if (!dir) return;
    setDraft(dir);
    commit(dir);
  }

  return (
    <div className="flex flex-col gap-2">
      <label htmlFor="repo-checkout" className="text-sm font-medium">
        Checkout
      </label>
      <div className="flex gap-2">
        <Input
          id="repo-checkout"
          className="h-8 min-w-0 flex-1 font-mono text-xs"
          placeholder="~/code/project"
          autoComplete="off"
          value={draft}
          aria-invalid={refusal ? true : undefined}
          aria-describedby="repo-checkout-help"
          onChange={(e) => setDraft(e.target.value)}
          onBlur={() => commit(draft)}
          onKeyDown={(e) => {
            if (e.key === "Enter") commit(draft);
          }}
        />
        <Button type="button" variant="outline" size="sm" onClick={() => void choose()}>
          <FolderOpenIcon data-icon="inline-start" />
          Choose…
        </Button>
      </div>
      <p
        id="repo-checkout-help"
        className={cn("text-body/4.5 wrap-anywhere text-muted-foreground", refusal && "text-destructive")}
      >
        {refusal ?? checkoutHelp(repo, saved)}
      </p>
    </div>
  );
}

function LimitRow({
  limit,
  pending,
  onChange,
}: {
  limit: number;
  pending: boolean;
  onChange: (limit: number) => void;
}) {
  const [draft, setDraft] = useState(String(limit));
  const [invalid, setInvalid] = useState(false);

  function commit() {
    if (pending) return;
    const wanted = wholeNumber(draft) ?? 0;
    const refused = wanted < 1;
    setInvalid(refused);
    if (refused || wanted === limit) return;
    onChange(wanted);
  }

  return (
    <SettingRow
      label="At the same time"
      htmlFor="repo-dependabot-limit"
      description={
        invalid ? (
          <span className="text-destructive">Use a whole number from 1.</span>
        ) : (
          "The rest wait in the queue, oldest first."
        )
      }
    >
      <Input
        id="repo-dependabot-limit"
        type="number"
        inputMode="numeric"
        min={1}
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

type WatchDefaultsProps = {
  overrides: WatchOverrides;
  pending: boolean;
  onChange: (overrides: WatchOverrides) => void;
};

function WatchDefaults({ overrides, pending, onChange }: WatchDefaultsProps) {
  const providers = useProviders(true);
  const settings = useSettings(true);
  const catalog = providers.data ?? [];
  const daemon = settings.data ? daemonDefaults(settings.data) : undefined;
  const effective = settings.data ? repositoryDefaults(settings.data, overrides) : undefined;
  const provider: Provider["id"] = overrides.provider || daemon?.provider || "claude";
  const models = catalog.find((item) => item.id === provider)?.models ?? [];
  const summary = effective
    ? [agentLabel(catalog, effective.provider, effective.model, effective.effort), effective.approvalMode].join(" · ")
    : "";
  const save = (next: Partial<WatchOverrides>) => onChange({ ...overrides, ...next });
  const inherited = <T,>(format: (defaults: Defaults) => T) => (daemon ? format(daemon) : undefined);
  const daemonProvider = inherited((d) => catalog.find((item) => item.id === d.provider)?.label ?? d.provider);
  const modelOptions: Option<string>[] = overrides.provider
    ? toOptions(models)
    : [{ value: "", label: defaultLabel(inherited((d) => modelLabel(catalog, d.provider, d.model))) }];
  const efforts = overrides.provider ? effortsOf(catalog, provider, overrides.model) : [];
  const effortDefault = effortDefaultLabel(catalog, overrides.provider ? null : daemon);

  return (
    <Collapsible className="border-t pt-2">
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          className="-mx-2 h-10 w-[calc(100%+1rem)] justify-start px-2 has-[>svg]:px-2"
        >
          <ChevronDownIcon
            data-icon="inline-start"
            className="-rotate-90 text-muted-foreground transition-transform [[data-state=open]>&]:rotate-0"
          />
          Watch defaults
          <span className="ml-auto flex min-w-0 items-center gap-1.5">
            <AgentLogo provider={effective?.provider ?? provider} className="size-3" />
            <Meta className="truncate">{summary}</Meta>
          </span>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-2 flex flex-col gap-4">
        <p className="text-body/4.5 text-muted-foreground">
          Each watch on this repository starts with these values, by hand or by auto start. A value equal to the setting
          of the daemon follows the daemon.
        </p>
        <SettingRow
          label="Agent"
          htmlFor="repo-override-provider"
          description="Prepares the fixes and the replies."
          className="flex-col items-stretch"
        >
          <div className="grid grid-cols-1 gap-2">
            <Select
              value={toSelectValue(overrides.provider)}
              disabled={pending || catalog.length === 0}
              onValueChange={(next) =>
                save({ provider: fromSelectValue(next) as WatchOverrides["provider"], model: "", effort: "" })
              }
            >
              <SelectTrigger id="repo-override-provider" size="sm" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={toSelectValue("")}>
                  {daemon ? <AgentLogo provider={daemon.provider} /> : null}
                  {defaultLabel(daemonProvider)}
                </SelectItem>
                {catalog.map((item) => (
                  <SelectItem key={item.id} value={item.id} disabled={!item.available}>
                    <AgentLogo provider={item.id} />
                    {item.label}
                    {item.available ? "" : " (command not found)"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <OptionSelect
              label="Model"
              className="w-full"
              options={modelOptions}
              value={overrides.model}
              disabled={pending || !overrides.provider || models.length === 0}
              onChange={(model) => save({ model, effort: "" })}
            />
          </div>
        </SettingRow>
        <SettingRow
          label="Effort"
          htmlFor="repo-override-effort"
          description="How much the model reasons before it acts."
        >
          <EffortSelect
            id="repo-override-effort"
            className={OVERRIDE_SELECT}
            efforts={efforts}
            defaultLabel={effortDefault}
            value={overrides.effort}
            disabled={pending}
            onChange={(effort) => save({ effort })}
          />
        </SettingRow>
        <SettingRow
          label="Approval mode"
          htmlFor="repo-override-approval-mode"
          description="Manual holds each turn until you approve it."
        >
          <OptionSelect
            id="repo-override-approval-mode"
            className={OVERRIDE_SELECT}
            options={[
              { value: "", label: defaultLabel(daemon?.approvalMode) },
              { value: "manual", label: "manual" },
              { value: "auto", label: "auto" },
            ]}
            value={overrides.approvalMode}
            disabled={pending}
            onChange={(approvalMode) => save({ approvalMode })}
          />
        </SettingRow>
        <SwitchOverride
          id="repo-override-auto-rebase"
          label="Approve a clean rebase or merge on its own"
          description="Approved work does not ask again because the branch moved. No effect in auto."
          checked={effective?.autoApproveRebase}
          disabled={pending || !daemon}
          onChange={(on) => save({ autoApproveRebase: overrideOf(on, daemon?.autoApproveRebase) })}
        />
        <SettingRow
          label="Merge method"
          htmlFor="repo-override-merge-method"
          description="Used by Merge in the header."
        >
          <OptionSelect
            id="repo-override-merge-method"
            className={OVERRIDE_SELECT}
            options={[
              { value: "", label: defaultLabel(inherited((d) => mergeMethodDefaultLabel(d.mergeMethod))) },
              ...(["squash", "merge", "rebase"] as const).map((value) => ({ value, label: mergeMethodLabel(value) })),
            ]}
            value={overrides.mergeMethod}
            disabled={pending}
            onChange={(mergeMethod) => save({ mergeMethod })}
          />
        </SettingRow>
        <SettingRow
          label="Branch behind its base"
          htmlFor="repo-override-branch-update"
          description="The agent solves a conflict the same way."
        >
          <OptionSelect
            id="repo-override-branch-update"
            className={OVERRIDE_SELECT}
            options={[
              { value: "", label: defaultLabel(inherited((d) => branchUpdateDefaultLabel(d.branchUpdate))) },
              ...BRANCH_UPDATES,
            ]}
            value={overrides.branchUpdate}
            disabled={pending}
            onChange={(branchUpdate) => save({ branchUpdate })}
          />
        </SettingRow>
        <SwitchOverride
          id="repo-override-update-on-github"
          label="Update the branch on GitHub first"
          description="The agent does it only when GitHub refuses."
          checked={effective?.updateOnGitHub}
          disabled={pending || !daemon}
          onChange={(on) => save({ updateOnGitHub: overrideOf(on, daemon?.updateOnGitHub) })}
        />
        {effective && daemon ? (
          <ApprovalsOverride
            key={String(effective.approvalsRequired)}
            value={effective.approvalsRequired}
            pending={pending}
            onChange={(approvals) => save({ approvalsRequired: overrideOf(approvals, daemon.approvalsRequired) })}
          />
        ) : null}
        <SwitchOverride
          id="repo-override-include-existing"
          label="Report existing review items"
          description="Items that were there before the watch started."
          checked={effective?.includeExisting}
          disabled={pending || !daemon}
          onChange={(on) => save({ includeExisting: overrideOf(on, daemon?.includeExisting) })}
        />
        <SwitchOverride
          id="repo-override-include-own"
          label="Report my own comments"
          description="Treat your comments like a reviewer's."
          checked={effective?.includeOwn}
          disabled={pending || !daemon}
          onChange={(on) => save({ includeOwn: overrideOf(on, daemon?.includeOwn) })}
        />
        <SwitchOverride
          id="repo-override-keep-worktree"
          label="Keep the worktree when a watch stops"
          description="The stop dialog can still say otherwise."
          checked={effective?.keepWorktree}
          disabled={pending || !daemon}
          onChange={(on) => save({ keepWorktree: overrideOf(on, daemon?.keepWorktree) })}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}

type SwitchOverrideProps = {
  id: string;
  label: string;
  description: string;
  checked: boolean | undefined;
  disabled: boolean;
  onChange: (on: boolean) => void;
};

function SwitchOverride({ id, label, description, checked, disabled, onChange }: SwitchOverrideProps) {
  return (
    <SettingRow label={label} htmlFor={id} description={description}>
      <Switch id={id} checked={checked ?? false} disabled={disabled} onCheckedChange={onChange} />
    </SettingRow>
  );
}

type ApprovalsOverrideProps = {
  value: number | null;
  pending: boolean;
  onChange: (value: number | null) => void;
};

function ApprovalsOverride({ value, pending, onChange }: ApprovalsOverrideProps) {
  const [draft, setDraft] = useState(approvalsField(value));
  const [invalid, setInvalid] = useState(false);

  function commit() {
    if (pending) return;
    const refused = approvalsInvalid(draft);
    setInvalid(refused);
    if (refused) return;
    const wanted = approvalsRequired(draft) ?? null;
    if (wanted === value) return;
    onChange(wanted);
  }

  return (
    <SettingRow
      label="Approvals before ready to merge"
      htmlFor="repo-override-approvals"
      description={
        invalid ? (
          <span className="text-destructive">Use a whole number from 0, or leave it empty.</span>
        ) : (
          "Empty takes the rule of the base branch."
        )
      }
    >
      <Input
        id="repo-override-approvals"
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
