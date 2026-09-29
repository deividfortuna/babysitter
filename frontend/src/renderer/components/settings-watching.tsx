import { useState } from "react";
import { CircleAlertIcon } from "lucide-react";
import { useProviders, type Provider } from "@/hooks/useProviders";
import { useSaveSettings, useSettings, type Settings } from "@/hooks/useSettings";
import { AgentLogo } from "@/components/agent-logo";
import { OptionSelect, toOptions } from "@/components/option-select";
import { EffortSelect } from "@/components/effort-select";
import { MergeMethodSelect } from "@/components/merge-method-select";
import { ApprovalModeSelect } from "@/components/approval-mode-select";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel, FieldTitle } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { approvalsField, approvalsInvalid, approvalsRequired, wholeNumber } from "@/lib/approvals";
import { effortDefaultLabel, effortsOf } from "@/lib/watch-defaults";

type Draft = Omit<Settings, "pollIntervalSeconds" | "watchIntervalSeconds" | "approvalsRequired"> & {
  pollIntervalSeconds: string;
  watchIntervalSeconds: string;
  approvalsRequired: string;
};

function toDraft(settings: Settings): Draft {
  return {
    ...settings,
    pollIntervalSeconds: String(settings.pollIntervalSeconds),
    watchIntervalSeconds: String(settings.watchIntervalSeconds),
    approvalsRequired: approvalsField(settings.approvalsRequired),
  };
}

function toSettings(draft: Draft): Settings {
  return {
    ...draft,
    pollIntervalSeconds: Number(draft.pollIntervalSeconds),
    watchIntervalSeconds: Number(draft.watchIntervalSeconds),
    approvalsRequired: approvalsRequired(draft.approvalsRequired) ?? null,
  };
}

function refusal(draft: Draft): string | null {
  const intervals = [draft.pollIntervalSeconds, draft.watchIntervalSeconds];
  if (intervals.some((field) => wholeNumber(field) === undefined)) {
    return "The poll intervals take a whole number of seconds.";
  }
  if (approvalsInvalid(draft.approvalsRequired)) {
    return "The approvals take a whole number, 0 or more.";
  }
  return null;
}

export function WatchingPanel({ onSaved }: { onSaved: () => void }) {
  const settings = useSettings();

  if (settings.isError) {
    return (
      <Alert variant="destructive">
        <CircleAlertIcon />
        <AlertTitle>{settings.error.message}</AlertTitle>
      </Alert>
    );
  }
  if (!settings.data) return <Spinner />;
  return <WatchingForm settings={settings.data} onSaved={onSaved} />;
}

function WatchingForm({ settings, onSaved }: { settings: Settings; onSaved: () => void }) {
  const save = useSaveSettings();
  const providers = useProviders(true);
  const [draft, setDraft] = useState(() => toDraft(settings));
  const [edited, setEdited] = useState(false);

  function edit(patch: Partial<Draft>) {
    setDraft((current) => ({ ...current, ...patch }));
    setEdited(true);
    save.reset();
  }

  const refused = refusal(draft);
  const efforts = effortsOf(providers.data ?? [], draft.provider, draft.model);

  function submit() {
    if (refused) return;
    save.mutate(toSettings(draft), { onSuccess: onSaved });
  }

  return (
    <div className="flex flex-col gap-6">
      <FieldGroup>
        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="poll-interval">Repository poll interval</FieldLabel>
            <FieldDescription>Seconds between passes over the repositories you watch.</FieldDescription>
          </FieldContent>
          <Input
            id="poll-interval"
            type="number"
            inputMode="numeric"
            className="w-24"
            value={draft.pollIntervalSeconds}
            onChange={(e) => edit({ pollIntervalSeconds: e.target.value })}
          />
        </Field>

        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="watch-interval">Watch poll interval</FieldLabel>
            <FieldDescription>Seconds between polls of a pull request under watch.</FieldDescription>
          </FieldContent>
          <Input
            id="watch-interval"
            type="number"
            inputMode="numeric"
            className="w-24"
            value={draft.watchIntervalSeconds}
            onChange={(e) => edit({ watchIntervalSeconds: e.target.value })}
          />
        </Field>
      </FieldGroup>

      <FieldGroup>
        <div className="flex flex-col gap-1">
          <FieldTitle className="text-muted-foreground">Defaults of a new watch</FieldTitle>
          <FieldDescription>A repository can set its own value, and a watch can set its own at start.</FieldDescription>
        </div>

        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="agent">Agent</FieldLabel>
            <FieldDescription>Prepares the fixes and the replies.</FieldDescription>
          </FieldContent>
          <AgentFields
            catalog={providers.data ?? []}
            provider={draft.provider}
            model={draft.model}
            onChange={(provider, model) => edit({ provider, model, effort: "" })}
          />
        </Field>

        <Field orientation="horizontal" data-disabled={efforts.length === 0 || undefined}>
          <FieldContent>
            <FieldLabel htmlFor="effort">Effort</FieldLabel>
            <FieldDescription>
              How much the model reasons before it acts. More effort is slower and uses more tokens.
            </FieldDescription>
          </FieldContent>
          <EffortSelect
            id="effort"
            size="default"
            className="w-38"
            efforts={efforts}
            defaultLabel={effortDefaultLabel(providers.data ?? [], null)}
            value={draft.effort}
            onChange={(effort) => edit({ effort })}
          />
        </Field>

        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="approval-mode">Approval mode</FieldLabel>
            <FieldDescription>
              Manual holds each turn of the agent until you read and approve it. Auto pushes and posts as soon as the
              turn ends. A watch your own coding session starts always runs in auto.
            </FieldDescription>
          </FieldContent>
          <ApprovalModeSelect
            id="approval-mode"
            value={draft.approvalMode}
            onChange={(approvalMode) => edit({ approvalMode })}
          />
        </Field>

        <Field orientation="horizontal" data-disabled={draft.approvalMode === "auto" || undefined}>
          <FieldContent>
            <FieldLabel htmlFor="auto-rebase">Approve a clean rebase on its own</FieldLabel>
            <FieldDescription>
              Work you approved does not ask again because the branch moved under it. A rebase that conflicts always
              asks. No effect in auto.
            </FieldDescription>
          </FieldContent>
          <Switch
            id="auto-rebase"
            checked={draft.autoApproveRebase}
            disabled={draft.approvalMode === "auto"}
            onCheckedChange={(on) => edit({ autoApproveRebase: on })}
          />
        </Field>

        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="approvals">Approvals before ready to merge</FieldLabel>
            <FieldDescription>Empty takes the rule of the base branch. Zero asks for no review.</FieldDescription>
          </FieldContent>
          <Input
            id="approvals"
            type="number"
            inputMode="numeric"
            className="w-24"
            value={draft.approvalsRequired}
            onChange={(e) => edit({ approvalsRequired: e.target.value })}
          />
        </Field>

        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="merge-method">Merge method</FieldLabel>
            <FieldDescription>Empty takes the first method the repository allows.</FieldDescription>
          </FieldContent>
          <MergeMethodSelect id="merge-method" value={draft.mergeMethod} onChange={(v) => edit({ mergeMethod: v })} />
        </Field>

        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="include-existing">Report the review items that already exist</FieldLabel>
            <FieldDescription>The first poll hands the agent what is on the pull request already.</FieldDescription>
          </FieldContent>
          <Switch
            id="include-existing"
            checked={draft.includeExisting}
            onCheckedChange={(on) => edit({ includeExisting: on })}
          />
        </Field>

        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="include-own">Report my own comments</FieldLabel>
            <FieldDescription>For a repository where you review your own work.</FieldDescription>
          </FieldContent>
          <Switch id="include-own" checked={draft.includeOwn} onCheckedChange={(on) => edit({ includeOwn: on })} />
        </Field>

        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="keep-worktree">Keep the worktree when a watch stops</FieldLabel>
            <FieldDescription>The worktree of the agent stays on disk instead of going away.</FieldDescription>
          </FieldContent>
          <Switch
            id="keep-worktree"
            checked={draft.keepWorktree}
            onCheckedChange={(on) => edit({ keepWorktree: on })}
          />
        </Field>
      </FieldGroup>

      {refused || save.error ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{refused ?? save.error?.message}</AlertTitle>
        </Alert>
      ) : null}

      <div className="flex items-center justify-end gap-3">
        <Button onClick={submit} disabled={!edited || save.isPending}>
          {save.isPending ? <Spinner /> : null}
          Save
        </Button>
      </div>
    </div>
  );
}

type AgentFieldsProps = {
  catalog: Provider[];
  provider: Settings["provider"];
  model: string;
  onChange: (provider: Settings["provider"], model: string) => void;
};

function AgentFields({ catalog, provider, model, onChange }: AgentFieldsProps) {
  const models = catalog.find((item) => item.id === provider)?.models ?? [];
  return (
    <div className="flex w-72 shrink-0 gap-2">
      <Select
        value={provider}
        disabled={catalog.length === 0}
        onValueChange={(next) => onChange(next as Settings["provider"], "")}
      >
        <SelectTrigger id="agent" className="w-32 shrink-0">
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
      <OptionSelect
        label="Model"
        size="default"
        className="min-w-0 flex-1"
        options={toOptions(models)}
        value={model}
        disabled={models.length === 0}
        onChange={(next) => onChange(provider, next)}
      />
    </div>
  );
}
