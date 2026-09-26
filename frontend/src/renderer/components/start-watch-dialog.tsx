import { useMemo, useState } from "react";
import { ChevronDownIcon, CircleAlertIcon, FolderOpenIcon, GitPullRequestIcon } from "lucide-react";
import { usePulls, type PullRequest } from "@/hooks/usePulls";
import { useProviders, type Provider } from "@/hooks/useProviders";
import type { ApprovalMode } from "@/hooks/useProposals";
import { useSettings } from "@/hooks/useSettings";
import { useStartWatch, useWatches, type MergeMethod, type Watch } from "@/hooks/useWatches";
import { AgentLogo } from "@/components/agent-logo";
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
import { MergeMethodSelect } from "@/components/merge-method-select";
import { ApprovalModeSelect } from "@/components/approval-mode-select";
import { approvalsField, approvalsInvalid, approvalsRequired } from "@/lib/approvals";
import { bridge } from "@/lib/bridge";
import { fromSelectValue, toSelectValue } from "@/lib/select-value";
import { settingsSummary } from "@/lib/start-watch-summary";

const DIR_KEY_PREFIX = "checkout_dir:";
const DIR_KEY_LAST = "checkout_dir";

const TARGET_RE = /^(https?:\/\/\S+|[\w.-]+\/[\w.-]+#\d+)$/;

const ROW = "gap-4 border-t py-3";
const CONTROL = "flex w-55 shrink-0 justify-end";

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
  provider: Provider["id"];
  model: string;
  approvalMode: ApprovalMode | null;
  autoRebase: boolean | null;
};

function startCommand(target: string, choices: StartChoices): string {
  const words = ["babysitter watch start", target];
  if (choices.provider !== "claude") words.push(`--provider ${choices.provider}`);
  if (choices.model) words.push(`--model ${choices.model}`);
  if (choices.approvalMode) words.push(`--approval-mode ${choices.approvalMode}`);
  if (choices.autoRebase !== null)
    words.push(choices.autoRebase ? "--auto-approve-rebase" : "--auto-approve-rebase=false");
  return words.join(" ");
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

function StartWatchForm({ enabled, initial, onStarted }: FormProps) {
  const pulls = usePulls(enabled);
  const watches = useWatches(enabled);
  const providers = useProviders(enabled);
  const settings = useSettings(enabled);
  const start = useStartWatch();

  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<PullRequest | null>(initial);
  const [sourceDir, setSourceDir] = useState(() => (initial ? readDir(initial.repo) : "") || readDir(null));
  const [rememberedFor, setRememberedFor] = useState(() => (initial && readDir(initial.repo) ? initial.repo : null));
  const [moreOpen, setMoreOpen] = useState(false);
  const [chosenProvider, setProvider] = useState<Provider["id"]>("claude");
  const [chosenModel, setModel] = useState("");
  const [includeExisting, setIncludeExisting] = useState<boolean | null>(null);
  const [includeOwn, setIncludeOwn] = useState<boolean | null>(null);
  const [approvals, setApprovals] = useState<string | null>(null);
  const [mergeMethod, setMergeMethod] = useState<MergeMethod | null>(null);
  const [approvalMode, setApprovalMode] = useState<ApprovalMode | null>(null);
  const [autoRebase, setAutoRebase] = useState<boolean | null>(null);

  const defaults = settings.data;
  const includeExistingValue = includeExisting ?? defaults?.includeExisting ?? false;
  const includeOwnValue = includeOwn ?? defaults?.includeOwn ?? false;
  const mergeMethodValue = mergeMethod ?? defaults?.mergeMethod ?? "";
  const approvalsValue = approvals ?? approvalsField(defaults?.approvalsRequired);
  const approvalModeValue = approvalMode ?? defaults?.approvalMode ?? "manual";
  const autoRebaseValue = autoRebase ?? defaults?.autoApproveRebase ?? false;
  const asks = approvalModeValue === "manual";
  const rebaseChosen = asks && autoRebase !== null;

  const catalog = useMemo(() => providers.data ?? [], [providers.data]);
  const provider = installedProvider(catalog, chosenProvider);
  const model = provider === chosenProvider ? chosenModel : "";
  const models = useMemo(() => catalog.find((item) => item.id === provider)?.models ?? [], [catalog, provider]);

  const watched = useMemo(() => new Set((watches.data ?? []).map((w) => `${w.repo}#${w.number}`)), [watches.data]);
  const openPulls = useMemo(
    () => (pulls.data ?? []).filter((pr) => pr.state === "open").sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)),
    [pulls.data],
  );

  const typed = query.trim();
  const byReference = !picked && TARGET_RE.test(typed);
  const target = picked ? `${picked.repo}#${picked.number}` : typed;
  const badApprovals = approvalsInvalid(approvalsValue);
  const canStart =
    Boolean((picked || byReference) && sourceDir.trim()) && !badApprovals && !start.isPending && settings.isSuccess;

  const agentLabel = catalog.find((item) => item.id === provider)?.label ?? provider;
  const modelLabel = model ? (models.find((item) => item.id === model)?.label ?? model) : "";
  const summary = settingsSummary({
    agent: agentLabel,
    model: modelLabel,
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
    const dir = await bridge.dialog.pickDirectory(sourceDir.trim() || undefined);
    if (dir) changeDir(dir);
  }

  function submit() {
    if (!canStart) return;
    start.mutate(
      {
        target,
        repo: "",
        provider,
        model,
        sourceDir: sourceDir.trim(),
        ...(includeExisting === null ? {} : { includeExisting }),
        ...(includeOwn === null ? {} : { includeOwn }),
        ...(approvals === null ? {} : { approvalsRequired: approvalsRequired(approvals) ?? null }),
        ...(mergeMethod === null ? {} : { mergeMethod }),
        ...(approvalMode === null ? {} : { approvalMode }),
        ...(rebaseChosen ? { autoApproveRebase: autoRebase } : {}),
      },
      {
        onSuccess: (watch) => {
          storeDir(watch.repo, sourceDir.trim());
          onStarted(watch);
        },
      },
    );
  }

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
              placeholder="~/code/project"
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
            {rememberedFor ? `Remembered for ${rememberedFor}. ` : null}A private worktree is made next to it; your
            checkout is never touched.
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
                <Meta className="truncate">{moreOpen ? "from your defaults" : summary}</Meta>
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
              <div className="flex w-80 shrink-0 gap-2">
                <Select
                  value={provider}
                  disabled={catalog.length === 0}
                  onValueChange={(next) => {
                    setProvider(next as Provider["id"]);
                    setModel("");
                  }}
                >
                  <SelectTrigger id="provider" className="w-32 shrink-0">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {catalog.map((item) => (
                      <SelectItem key={item.id} value={item.id} disabled={!item.available}>
                        <AgentLogo provider={item.id} />
                        {item.label}
                        {item.available ? "" : " (command not found)"}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select
                  value={toSelectValue(model)}
                  disabled={models.length === 0}
                  onValueChange={(next) => setModel(fromSelectValue(next))}
                >
                  <SelectTrigger aria-label="Model" className="min-w-0 flex-1">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {models.map((item) => (
                      <SelectItem key={item.id} value={toSelectValue(item.id)}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
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
                <ApprovalModeSelect
                  id="approval-mode"
                  className="w-full"
                  value={approvalModeValue}
                  onChange={setApprovalMode}
                />
              </div>
            </SettingRow>

            <SettingRow
              label="Approve a clean rebase on its own"
              htmlFor="auto-rebase"
              description="Approved work does not ask again because the branch moved."
              className={ROW}
            >
              <div className={CONTROL}>
                <Switch
                  id="auto-rebase"
                  checked={asks && autoRebaseValue}
                  disabled={!asks}
                  onCheckedChange={setAutoRebase}
                />
              </div>
            </SettingRow>

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
                <MergeMethodSelect
                  id="merge-method"
                  className="w-full"
                  value={mergeMethodValue}
                  onChange={setMergeMethod}
                />
              </div>
            </SettingRow>

            <SettingRow
              label="Report existing review items"
              htmlFor="include-existing"
              description="Items that were there before the watch started."
              className={ROW}
            >
              <div className={CONTROL}>
                <Switch id="include-existing" checked={includeExistingValue} onCheckedChange={setIncludeExisting} />
              </div>
            </SettingRow>

            <SettingRow
              label="Include my own comments"
              htmlFor="include-own"
              description="Treat your comments like a reviewer's."
              className={ROW}
            >
              <div className={CONTROL}>
                <Switch id="include-own" checked={includeOwnValue} onCheckedChange={setIncludeOwn} />
              </div>
            </SettingRow>
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
              provider,
              model,
              approvalMode,
              autoRebase: rebaseChosen ? autoRebaseValue : null,
            })}
          </Meta>
        ) : null}
      </DialogFooter>
    </>
  );
}
