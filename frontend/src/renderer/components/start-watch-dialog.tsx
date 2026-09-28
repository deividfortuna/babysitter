import { useMemo, useState } from "react";
import { ChevronDownIcon, CircleAlertIcon, FolderOpenIcon, GitPullRequestIcon } from "lucide-react";
import { usePulls, type PullRequest } from "@/hooks/usePulls";
import { useProviders, type Provider } from "@/hooks/useProviders";
import type { ApprovalMode } from "@/hooks/useProposals";
import { useSettings } from "@/hooks/useSettings";
import { useRepoConfig, useRepos } from "@/hooks/useRepos";
import { useStartWatch, useWatches, type MergeMethod, type Watch } from "@/hooks/useWatches";
import { AgentLogo } from "@/components/agent-logo";
import { OptionSelect, type Option } from "@/components/option-select";
import { SettingRow } from "@/components/setting-row";
import { Meta } from "@/components/status-badges";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { mergeMethodLabel } from "@/components/merge-method-select";
import { approvalsField, approvalsInvalid, approvalsRequired } from "@/lib/approvals";
import { bridge } from "@/lib/bridge";
import { fromSelectValue, toSelectValue } from "@/lib/select-value";
import { settingsSummary } from "@/lib/start-watch-summary";
import { defaultLabel, mergeMethodDefaultLabel, modelLabel, repositoryDefaults } from "@/lib/watch-defaults";

const DIR_KEY_PREFIX = "checkout_dir:";
const DIR_KEY_LAST = "checkout_dir";

const TARGET_RE = /^(https?:\/\/\S+|[\w.-]+\/[\w.-]+#\d+)$/;

const ROW = "gap-4 border-t py-3";
const CONTROL = "flex w-55 shrink-0 justify-end";

const DEFAULT_SOURCE = "Default takes the repository, then the daemon";

function readDir(repo: string | null): string {
  try {
    return window.localStorage.getItem(repo ? DIR_KEY_PREFIX + repo : DIR_KEY_LAST) ?? "";
  } catch {
    return "";
  }
}

function storeDir(repo: string, dir: string): void {
  try {
    window.localStorage.setItem(DIR_KEY_PREFIX + repo, dir);
    window.localStorage.setItem(DIR_KEY_LAST, dir);
  } catch {}
}

type StartChoices = {
  provider: Provider["id"] | null;
  model: string | null;
  approvalMode: ApprovalMode | null;
  autoRebase: boolean | null;
  mergeWhenReady: boolean;
  noCheckout: boolean;
};

function startCommand(target: string, choices: StartChoices): string {
  const words = ["babysitter watch start", target];
  if (choices.provider) words.push(`--provider ${choices.provider}`);
  if (choices.model) words.push(`--model ${choices.model}`);
  if (choices.approvalMode) words.push(`--approval-mode ${choices.approvalMode}`);
  if (choices.autoRebase !== null)
    words.push(choices.autoRebase ? "--auto-approve-rebase" : "--auto-approve-rebase=false");
  if (choices.mergeWhenReady) words.push("--merge-when-ready");
  if (choices.noCheckout) words.push("--no-checkout");
  return words.join(" ");
}

const REPO_OF_URL = /github\.com\/([\w.-]+\/[\w.-]+)\/pull\/\d+/;
const REPO_OF_REFERENCE = /^([\w.-]+\/[\w.-]+)#\d+$/;

function repoOfTarget(target: string): string | null {
  return REPO_OF_URL.exec(target)?.[1] ?? REPO_OF_REFERENCE.exec(target)?.[1] ?? null;
}

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  enabled: boolean;
  initial?: PullRequest | null;
  onStarted: (watch: Watch) => void;
};

function installedProvider(catalog: Provider[], wanted: Provider["id"]): Provider["id"] {
  const wantedIsInstalled = catalog.find((item) => item.id === wanted)?.available ?? false;
  if (catalog.length === 0 || wantedIsInstalled) return wanted;
  return (catalog.find((item) => item.available) ?? catalog[0]).id;
}

export function StartWatchDialog({ open, onOpenChange, enabled, initial, onStarted }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex w-(--size-dialog-wide) flex-col gap-0 overflow-hidden p-0 sm:max-w-(--size-dialog-max)">
        <StartWatchForm
          enabled={enabled}
          initial={initial ?? null}
          onStarted={(watch) => {
            onOpenChange(false);
            onStarted(watch);
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

type FormProps = {
  enabled: boolean;
  initial: PullRequest | null;
  onStarted: (watch: Watch) => void;
};

type MergeChoice = "" | "first" | "squash" | "merge" | "rebase";

function mergeMethodOf(choice: MergeChoice): MergeMethod {
  return choice === "first" ? "" : choice;
}

function StartWatchForm({ enabled, initial, onStarted }: FormProps) {
  const pulls = usePulls(enabled);
  const watches = useWatches(enabled);
  const providers = useProviders(enabled);
  const settings = useSettings(enabled);
  const repos = useRepos(enabled);
  const start = useStartWatch();

  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<PullRequest | null>(initial);
  const [sourceDir, setSourceDir] = useState(() => (initial ? readDir(initial.repo) : "") || readDir(null));
  const [rememberedFor, setRememberedFor] = useState(() => (initial && readDir(initial.repo) ? initial.repo : null));
  const [moreOpen, setMoreOpen] = useState(false);
  const [chosenProvider, setProvider] = useState<Provider["id"] | null>(null);
  const [chosenModel, setModel] = useState<string | null>(null);
  const [includeExisting, setIncludeExisting] = useState<boolean | null>(null);
  const [includeOwn, setIncludeOwn] = useState<boolean | null>(null);
  const [keepWorktree, setKeepWorktree] = useState<boolean | null>(null);
  const [approvals, setApprovals] = useState<string | null>(null);
  const [mergeMethod, setMergeMethod] = useState<MergeChoice>("");
  const [approvalMode, setApprovalMode] = useState<ApprovalMode | "">("");
  const [autoRebase, setAutoRebase] = useState<boolean | null>(null);
  const [mergeWhenReady, setMergeWhenReady] = useState<boolean | null>(null);

  const typed = query.trim();
  const byReference = !picked && TARGET_RE.test(typed);
  const target = picked ? `${picked.repo}#${picked.number}` : typed;
  const targetRepo = picked?.repo ?? repoOfTarget(typed);
  const repoId = repos.data?.find((repo) => repo.fullName.toLowerCase() === targetRepo?.toLowerCase())?.id ?? null;
  const repoConfig = useRepoConfig(repoId);
  const defaults = settings.data ? repositoryDefaults(settings.data, repoConfig.data?.overrides) : undefined;
  const repoSettingsLanded = repos.isSuccess && (repoId === null || repoConfig.isSuccess);
  const repoSettingsFailed = repos.isError || repoConfig.isError;
  const defaultsLanded = settings.isSuccess && repoSettingsLanded;

  const catalog = useMemo(() => providers.data ?? [], [providers.data]);
  const inheritedProvider = defaults?.provider ?? "claude";
  const installedFallback = installedProvider(catalog, inheritedProvider);
  const providerChoice = chosenProvider ?? (installedFallback === inheritedProvider ? null : installedFallback);
  const provider = providerChoice ?? inheritedProvider;
  const model = chosenModel ?? (providerChoice ? "" : (defaults?.model ?? ""));
  const models = catalog.find((item) => item.id === provider)?.models ?? [];
  const modelOptions: Option<string>[] = providerChoice
    ? models.map((item) => ({ value: item.id, label: item.label }))
    : [
        { value: "", label: defaultLabel(defaults && modelLabel(catalog, defaults.provider, defaults.model)) },
        ...models.filter((item) => item.id !== "").map((item) => ({ value: item.id, label: item.label })),
      ];

  const approvalModeValue = approvalMode || defaults?.approvalMode || "manual";
  const asks = approvalModeValue === "manual";
  const rebaseChosen = asks && autoRebase !== null;
  const autoRebaseValue = autoRebase ?? defaults?.autoApproveRebase ?? false;
  const includeExistingValue = includeExisting ?? defaults?.includeExisting ?? false;
  const includeOwnValue = includeOwn ?? defaults?.includeOwn ?? false;
  const keepWorktreeValue = keepWorktree ?? defaults?.keepWorktree ?? false;
  const mergeMethodValue = mergeMethod ? mergeMethodOf(mergeMethod) : (defaults?.mergeMethod ?? "");
  const approvalsValue = approvals ?? approvalsField(defaults?.approvalsRequired);
  const badApprovals = approvalsInvalid(approvalsValue);

  const watched = useMemo(() => new Set((watches.data ?? []).map((w) => `${w.repo}#${w.number}`)), [watches.data]);
  const openPulls = useMemo(
    () => (pulls.data ?? []).filter((pr) => pr.state === "open").sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)),
    [pulls.data],
  );

  const checkout = sourceDir.trim();
  const canStart = Boolean(picked || byReference) && !badApprovals && !start.isPending && defaultsLanded;

  const summary = settingsSummary({
    agent: catalog.find((item) => item.id === provider)?.label ?? provider,
    model: model ? (models.find((item) => item.id === model)?.label ?? model) : "",
    approvalMode: approvalModeValue,
    approvals: approvalsValue,
    mergeMethod: mergeMethodValue,
  });

  function pick(pr: PullRequest) {
    setPicked(pr);
    setQuery("");
    const remembered = readDir(pr.repo);
    setRememberedFor(remembered ? pr.repo : null);
    if (remembered) setSourceDir(remembered);
  }

  function unpick() {
    setPicked(null);
    setRememberedFor(null);
  }

  function changeDir(dir: string) {
    setSourceDir(dir);
    setRememberedFor(null);
  }

  async function choose() {
    const dir = await bridge.dialog.pickDirectory(checkout || undefined);
    if (dir) changeDir(dir);
  }

  function submit() {
    if (!canStart) return;
    start.mutate(
      {
        target,
        repo: "",
        ...(checkout ? { sourceDir: checkout } : {}),
        ...(providerChoice ? { provider: providerChoice } : {}),
        ...(chosenModel ? { model: chosenModel } : {}),
        ...(includeExisting === null ? {} : { includeExisting }),
        ...(includeOwn === null ? {} : { includeOwn }),
        ...(keepWorktree === null ? {} : { keepWorktree }),
        ...(approvals === null ? {} : { approvalsRequired: approvalsRequired(approvals) ?? null }),
        ...(mergeMethod ? { mergeMethod: mergeMethodOf(mergeMethod) } : {}),
        ...(approvalMode ? { approvalMode } : {}),
        ...(rebaseChosen ? { autoApproveRebase: autoRebase } : {}),
        ...(mergeWhenReady === null ? {} : { mergeWhenReady }),
      },
      {
        onSuccess: (watch) => {
          storeDir(watch.repo, checkout);
          onStarted(watch);
        },
      },
    );
  }

  const inheritedMergeMethod = defaults && mergeMethodDefaultLabel(defaults.mergeMethod);
  const inheritedProviderLabel = catalog.find((item) => item.id === inheritedProvider)?.label ?? inheritedProvider;

  return (
    <>
      <DialogHeader className="shrink-0 border-b p-6">
        <DialogTitle>Watch a pull request</DialogTitle>
        <DialogDescription>
          Pick one of your open pull requests, or paste a URL. It is checked every few minutes and only interrupts you
          for a decision.
        </DialogDescription>
      </DialogHeader>

      <FieldGroup className="gap-6 overflow-y-auto p-6">
        <Field>
          <FieldLabel className="eyebrow">Pull request</FieldLabel>
          {picked ? (
            <div className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm">
              <GitPullRequestIcon className="size-4 shrink-0 text-muted-foreground" />
              <span className="truncate">
                #{picked.number} {picked.title}
              </span>
              <Meta className="shrink-0">
                {picked.repo} · {picked.author}
              </Meta>
              <Button type="button" variant="ghost" size="xs" className="ml-auto shrink-0" onClick={unpick}>
                Change
              </Button>
            </div>
          ) : (
            <Command className="rounded-md border">
              <CommandInput placeholder="search your open PRs, or paste a URL" value={query} onValueChange={setQuery} />
              <CommandList className="max-h-44">
                <CommandEmpty className="px-3 py-4 text-left text-sm text-muted-foreground">
                  {byReference
                    ? `Watch ${typed} by reference.`
                    : pulls.isPending
                      ? "Loading the open pull requests…"
                      : "No open pull request matches. Paste a URL or owner/name#number."}
                </CommandEmpty>
                <CommandGroup>
                  {openPulls.map((pr) => {
                    const already = watched.has(`${pr.repo}#${pr.number}`);
                    return (
                      <CommandItem
                        key={`${pr.repo}#${pr.number}`}
                        value={`${pr.repo}#${pr.number} ${pr.title} ${pr.author}`}
                        disabled={already}
                        onSelect={() => pick(pr)}
                      >
                        <span className="min-w-0 flex-1 truncate">
                          #{pr.number} {pr.title}
                        </span>
                        <Meta className="max-w-[45%] shrink-0 truncate">
                          {pr.repo} · {already ? "already watched" : pr.author}
                        </Meta>
                      </CommandItem>
                    );
                  })}
                </CommandGroup>
              </CommandList>
            </Command>
          )}
        </Field>

        <Field>
          <FieldLabel htmlFor="source-dir" className="eyebrow">
            Checkout to copy the worktree from
          </FieldLabel>
          <div className="flex gap-2">
            <Input
              id="source-dir"
              className="font-mono text-xs"
              placeholder="Optional, e.g. ~/code/project"
              autoComplete="off"
              value={sourceDir}
              onChange={(e) => changeDir(e.target.value)}
            />
            <Button type="button" variant="outline" onClick={() => void choose()}>
              <FolderOpenIcon data-icon="inline-start" />
              Choose…
            </Button>
          </div>
          <FieldDescription>
            {rememberedFor ? `Remembered for ${rememberedFor}. ` : null}
            {checkout
              ? "A private worktree is made next to it; your checkout is never touched."
              : "Empty: babysitter clones the repository once into its data folder and makes the private worktree from that clone."}
          </FieldDescription>
        </Field>

        <Collapsible open={moreOpen} onOpenChange={setMoreOpen} className="-mt-1 border-t pt-3">
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
              Additional settings
              <span className="ml-auto flex min-w-0 items-center gap-1.5">
                {moreOpen ? null : <AgentLogo provider={provider} className="size-3" />}
                <Meta className="truncate">{moreOpen ? DEFAULT_SOURCE : summary}</Meta>
              </span>
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="mt-2">
            <SettingRow
              label="Agent"
              htmlFor="provider"
              description="Prepares the fixes and the replies."
              className={ROW}
            >
              <div className="flex w-96 shrink-0 gap-2">
                <Select
                  value={toSelectValue(providerChoice ?? "")}
                  disabled={catalog.length === 0}
                  onValueChange={(next) => {
                    setProvider((fromSelectValue(next) || null) as Provider["id"] | null);
                    setModel(null);
                  }}
                >
                  <SelectTrigger id="provider" className="min-w-0 flex-1">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={toSelectValue("")}>
                      <AgentLogo provider={inheritedProvider} />
                      {defaultLabel(inheritedProviderLabel)}
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
                  size="default"
                  className="min-w-0 flex-1"
                  options={modelOptions}
                  value={chosenModel ?? ""}
                  disabled={modelOptions.length <= 1}
                  onChange={(next) => setModel(providerChoice || next ? next : null)}
                />
              </div>
            </SettingRow>

            <SettingRow
              label="Approval mode"
              htmlFor="approval-mode"
              description={
                asks
                  ? "Manual holds each turn until you approve it. Nothing goes out before."
                  : "Auto pushes and posts as soon as each turn of the agent ends."
              }
              className={ROW}
            >
              <div className={CONTROL}>
                <OptionSelect
                  id="approval-mode"
                  size="default"
                  className="w-full"
                  options={[
                    { value: "", label: defaultLabel(defaults?.approvalMode) },
                    { value: "manual", label: "manual" },
                    { value: "auto", label: "auto" },
                  ]}
                  value={approvalMode}
                  onChange={setApprovalMode}
                />
              </div>
            </SettingRow>

            <SwitchRow
              id="auto-rebase"
              label="Approve a clean rebase on its own"
              description="Approved work does not ask again because the branch moved."
              checked={asks && autoRebaseValue}
              disabled={!asks}
              onChange={setAutoRebase}
            />

            <SettingRow
              label="Approvals before ready to merge"
              htmlFor="approvals"
              description={
                badApprovals ? (
                  <span className="text-destructive">Use a whole number from 0, or leave it empty.</span>
                ) : (
                  "Empty takes the rule of the base branch."
                )
              }
              className={ROW}
            >
              <div className={CONTROL}>
                <Input
                  id="approvals"
                  type="number"
                  min={0}
                  inputMode="numeric"
                  className="w-18"
                  value={approvalsValue}
                  onChange={(e) => setApprovals(e.target.value)}
                  aria-invalid={badApprovals || undefined}
                />
              </div>
            </SettingRow>

            <SettingRow
              label="Merge method"
              htmlFor="merge-method"
              description="Used by Merge in the header."
              className={ROW}
            >
              <div className={CONTROL}>
                <OptionSelect
                  id="merge-method"
                  size="default"
                  className="w-full"
                  options={[
                    { value: "", label: defaultLabel(inheritedMergeMethod) },
                    { value: "first", label: mergeMethodLabel("") },
                    ...(["squash", "merge", "rebase"] as const).map((value) => ({
                      value,
                      label: mergeMethodLabel(value),
                    })),
                  ]}
                  value={mergeMethod}
                  onChange={setMergeMethod}
                />
              </div>
            </SettingRow>

            <SettingRow
              label="Merge when ready"
              htmlFor="merge-when-ready"
              description="Merge with the method above as soon as the watch is ready to merge."
              className={ROW}
            >
              <div className={CONTROL}>
                <Switch id="merge-when-ready" checked={mergeWhenReady ?? false} onCheckedChange={setMergeWhenReady} />
              </div>
            </SettingRow>

            <SwitchRow
              id="include-existing"
              label="Report existing review items"
              description="Items that were there before the watch started."
              checked={includeExistingValue}
              onChange={setIncludeExisting}
            />

            <SwitchRow
              id="include-own"
              label="Include my own comments"
              description="Treat your comments like a reviewer's."
              checked={includeOwnValue}
              onChange={setIncludeOwn}
            />

            <SwitchRow
              id="keep-worktree"
              label="Keep the worktree when the watch stops"
              description="The stop dialog can still say otherwise."
              checked={keepWorktreeValue}
              onChange={setKeepWorktree}
            />
          </CollapsibleContent>
        </Collapsible>

        {settings.isError ? (
          <Alert variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>
              The settings of the daemon could not be read, so this dialog cannot say what a watch would take.
            </AlertTitle>
          </Alert>
        ) : null}

        {repoSettingsFailed ? (
          <Alert variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>
              The settings of the repository could not be read, so this dialog cannot say what a watch would take.
            </AlertTitle>
          </Alert>
        ) : null}

        {start.error ? (
          <Alert variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>{start.error.message}</AlertTitle>
          </Alert>
        ) : null}
      </FieldGroup>

      <DialogFooter className="shrink-0 border-t p-6 sm:justify-start">
        <Button type="button" onClick={submit} disabled={!canStart}>
          {start.isPending ? <Spinner data-icon="inline-start" /> : null}
          Start watching
        </Button>
        <DialogClose asChild>
          <Button type="button" variant="ghost">
            Cancel
          </Button>
        </DialogClose>
        {target ? (
          <Meta className="self-center sm:ml-auto">
            {startCommand(target, {
              provider: providerChoice,
              model: chosenModel,
              approvalMode: approvalMode || null,
              autoRebase: rebaseChosen ? autoRebaseValue : null,
              mergeWhenReady: mergeWhenReady ?? false,
              noCheckout: !checkout,
            })}
          </Meta>
        ) : null}
      </DialogFooter>
    </>
  );
}

type SwitchRowProps = {
  id: string;
  label: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (value: boolean) => void;
};

function SwitchRow({ id, label, description, checked, disabled, onChange }: SwitchRowProps) {
  return (
    <SettingRow label={label} htmlFor={id} description={description} className={ROW}>
      <div className={CONTROL}>
        <Switch id={id} checked={checked} disabled={disabled} onCheckedChange={onChange} />
      </div>
    </SettingRow>
  );
}
