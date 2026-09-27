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
import { bridge } from "@/lib/bridge";
import { fromSelectValue, toSelectValue } from "@/lib/select-value";
import { shortDate } from "@/lib/time";
import { cn } from "@/lib/utils";

type Option<T extends string> = { value: T; label: string };

const DAEMON_SETTING = "Daemon setting";
const NARROW_SELECT = "w-26 shrink-0 font-mono text-xs";
const OVERRIDE_SELECT = "w-32 shrink-0";

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

const APPROVAL_MODES: Option<WatchOverrides["approvalMode"]>[] = [
  { value: "", label: DAEMON_SETTING },
  { value: "manual", label: "manual" },
  { value: "auto", label: "auto" },
];

const MERGE_METHODS: Option<WatchOverrides["mergeMethod"]>[] = [
  { value: "", label: DAEMON_SETTING },
  ...(["squash", "merge", "rebase"] as const).map((value) => ({ value, label: mergeMethodLabel(value) })),
];

type ReportChoice = "" | "on" | "off";

const REPORT_CHOICES: Option<ReportChoice>[] = [
  { value: "", label: DAEMON_SETTING },
  { value: "on", label: "Report them" },
  { value: "off", label: "Skip them" },
];

type ApprovalsChoice = "" | "branch" | "count";

const APPROVALS_CHOICES: Option<ApprovalsChoice>[] = [
  { value: "", label: DAEMON_SETTING },
  { value: "branch", label: "Branch rule" },
  { value: "count", label: "A number" },
];

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
  const hasCheckout = config.checkoutDir !== "";
  const togglesLocked = !hasCheckout || toggles.isPending;

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
            disabled={togglesLocked}
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
            disabled={togglesLocked}
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
            disabled={togglesLocked}
            onCheckedChange={(autoWatchDependabot) => toggles.mutate({ autoWatchDependabot })}
          />
        </SettingRow>
      </Section>

      {hasCheckout ? (
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
      ) : null}

      <WatchesItStarts
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
        {refusal ?? `Auto start makes each worktree from it. It must have a remote for ${repo.fullName}.`}
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

type OptionSelectProps<T extends string> = {
  id?: string;
  className?: string;
  label?: string;
  options: Option<T>[];
  value: T;
  disabled?: boolean;
  onChange: (value: T) => void;
};

function OptionSelect<T extends string>({
  id,
  className,
  label,
  options,
  value,
  disabled,
  onChange,
}: OptionSelectProps<T>) {
  return (
    <Select
      value={toSelectValue(value)}
      disabled={disabled}
      onValueChange={(next) => onChange(fromSelectValue(next) as T)}
    >
      <SelectTrigger id={id} size="sm" aria-label={label} className={className}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={toSelectValue(option.value)}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function reportChoice(value: boolean | null | undefined): ReportChoice {
  if (value === true) return "on";
  if (value === false) return "off";
  return "";
}

function reportValue(choice: ReportChoice): boolean | undefined {
  if (choice === "") return undefined;
  return choice === "on";
}

function approvalsChoice(value: number | null | undefined): ApprovalsChoice {
  if (value === undefined) return "";
  if (value === null) return "branch";
  return "count";
}

function approvalsOfChoice(choice: ApprovalsChoice, current: number | null | undefined): number | null | undefined {
  if (choice === "") return undefined;
  if (choice === "branch") return null;
  return typeof current === "number" ? current : 1;
}

type WatchesItStartsProps = {
  overrides: WatchOverrides;
  pending: boolean;
  onChange: (overrides: WatchOverrides) => void;
};

function WatchesItStarts({ overrides, pending, onChange }: WatchesItStartsProps) {
  const providers = useProviders(true);
  const settings = useSettings(true);
  const catalog = providers.data ?? [];
  const provider: Provider["id"] = overrides.provider || "claude";
  const current = catalog.find((item) => item.id === provider);
  const models = current?.models ?? [];
  const agentLabel = current?.label ?? provider;
  const modelLabel = overrides.model
    ? (models.find((item) => item.id === overrides.model)?.label ?? overrides.model)
    : "";
  const approvalMode = overrides.approvalMode || settings.data?.approvalMode || "manual";
  const summary = [modelLabel ? `${agentLabel} ${modelLabel}` : agentLabel, approvalMode].join(" · ");
  const save = (next: Partial<WatchOverrides>) => onChange({ ...overrides, ...next });

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
          Watches it starts
          <span className="ml-auto flex min-w-0 items-center gap-1.5">
            <AgentLogo provider={provider} className="size-3" />
            <Meta className="truncate">{summary}</Meta>
          </span>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-2 flex flex-col gap-4">
        <SettingRow
          label="Agent"
          htmlFor="repo-override-provider"
          description="Prepares the fixes and the replies."
          className="flex-col items-stretch"
        >
          <div className="flex gap-2">
            <Select
              value={toSelectValue(overrides.provider)}
              disabled={pending || catalog.length === 0}
              onValueChange={(next) =>
                save({ provider: fromSelectValue(next) as WatchOverrides["provider"], model: "" })
              }
            >
              <SelectTrigger id="repo-override-provider" size="sm" className={OVERRIDE_SELECT}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={toSelectValue("")}>{DAEMON_SETTING}</SelectItem>
                {catalog.map((item) => (
                  <SelectItem key={item.id} value={item.id}>
                    <AgentLogo provider={item.id} />
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <OptionSelect
              label="Model"
              className="min-w-0 flex-1"
              options={models.map((item) => ({ value: item.id, label: item.label }))}
              value={overrides.model}
              disabled={pending || !overrides.provider || models.length === 0}
              onChange={(model) => save({ model })}
            />
          </div>
        </SettingRow>
        <SettingRow
          label="Approval mode"
          htmlFor="repo-override-approval-mode"
          description="Manual holds each turn until you approve it."
        >
          <OptionSelect
            id="repo-override-approval-mode"
            className={OVERRIDE_SELECT}
            options={APPROVAL_MODES}
            value={overrides.approvalMode}
            disabled={pending}
            onChange={(approvalMode) => save({ approvalMode })}
          />
        </SettingRow>
        <SettingRow
          label="Merge method"
          htmlFor="repo-override-merge-method"
          description="Used by Merge in the header."
        >
          <OptionSelect
            id="repo-override-merge-method"
            className={OVERRIDE_SELECT}
            options={MERGE_METHODS}
            value={overrides.mergeMethod}
            disabled={pending}
            onChange={(mergeMethod) => save({ mergeMethod })}
          />
        </SettingRow>
        <ApprovalsOverride
          key={String(overrides.approvalsRequired)}
          value={overrides.approvalsRequired}
          pending={pending}
          onChange={(approvalsRequired) => save({ approvalsRequired })}
        />
        <SettingRow
          label="Report existing review items"
          htmlFor="repo-override-include-existing"
          description="Items that were there before the watch started."
        >
          <OptionSelect
            id="repo-override-include-existing"
            className={OVERRIDE_SELECT}
            options={REPORT_CHOICES}
            value={reportChoice(overrides.includeExisting)}
            disabled={pending}
            onChange={(choice) => save({ includeExisting: reportValue(choice) })}
          />
        </SettingRow>
      </CollapsibleContent>
    </Collapsible>
  );
}

type ApprovalsOverrideProps = {
  value: number | null | undefined;
  pending: boolean;
  onChange: (value: number | null | undefined) => void;
};

function ApprovalsOverride({ value, pending, onChange }: ApprovalsOverrideProps) {
  const [draft, setDraft] = useState(approvalsField(value));
  const [invalid, setInvalid] = useState(false);
  const choice = approvalsChoice(value);

  function pick(next: ApprovalsChoice) {
    onChange(approvalsOfChoice(next, value));
  }

  function commit() {
    if (pending) return;
    const wanted = approvalsRequired(draft);
    const refused = approvalsInvalid(draft) || wanted === undefined;
    setInvalid(refused);
    if (refused || wanted === value) return;
    onChange(wanted);
  }

  return (
    <SettingRow
      label="Approvals before ready to merge"
      htmlFor="repo-override-approvals"
      description={
        invalid ? (
          <span className="text-destructive">Use a whole number from 0.</span>
        ) : (
          "The rule of the base branch, or a number."
        )
      }
      className="flex-col items-stretch"
    >
      <div className="flex gap-2">
        <OptionSelect
          id="repo-override-approvals"
          className={OVERRIDE_SELECT}
          options={APPROVALS_CHOICES}
          value={choice}
          disabled={pending}
          onChange={pick}
        />
        {choice === "count" ? (
          <Input
            type="number"
            inputMode="numeric"
            min={0}
            aria-label="Approvals"
            className="h-8 w-18 shrink-0"
            value={draft}
            aria-invalid={invalid || undefined}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commit}
            onKeyDown={(e) => {
              if (e.key === "Enter") commit();
            }}
          />
        ) : null}
      </div>
    </SettingRow>
  );
}
