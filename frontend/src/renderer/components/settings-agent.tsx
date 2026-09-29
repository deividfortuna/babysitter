import { HandIcon, ZapIcon, type LucideIcon } from "lucide-react";
import { AgentLogo } from "@/components/agent-logo";
import { EffortSelect } from "@/components/effort-select";
import { OptionSelect, toOptions } from "@/components/option-select";
import {
  DaemonSettings,
  SettingsCard,
  SettingsRow,
  SettingsSection,
  useTrackedWrite,
} from "@/components/settings-page";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import type { ApprovalMode } from "@/hooks/useProposals";
import { useProviders, type Provider } from "@/hooks/useProviders";
import type { Settings } from "@/hooks/useSettings";
import { effortDefaultLabel, effortsOf } from "@/lib/watch-defaults";

type ModeChoice = {
  mode: ApprovalMode;
  label: string;
  description: string;
  Icon: LucideIcon;
};

const MODE_CHOICES: ModeChoice[] = [
  {
    mode: "manual",
    label: "Manual",
    description: "Each turn waits for you. You read the diff and the replies, then approve.",
    Icon: HandIcon,
  },
  {
    mode: "auto",
    label: "Auto",
    description: "The agent pushes and posts as soon as its turn ends.",
    Icon: ZapIcon,
  },
];

export function AgentPanel() {
  return <DaemonSettings>{(settings) => <AgentForm settings={settings} />}</DaemonSettings>;
}

function AgentForm({ settings }: { settings: Settings }) {
  const save = useTrackedWrite();
  const providers = useProviders(true);
  const catalog = providers.data ?? [];
  const efforts = effortsOf(catalog, settings.provider, settings.model);
  const auto = settings.approvalMode === "auto";

  return (
    <div className="flex flex-col gap-4.5">
      <SettingsSection label="Who has the last word">
        <div role="radiogroup" aria-label="Approval mode" className="flex gap-2.5">
          {MODE_CHOICES.map((choice) => (
            <ModeTile
              key={choice.mode}
              choice={choice}
              checked={settings.approvalMode === choice.mode}
              onSelect={() => save({ approvalMode: choice.mode })}
            />
          ))}
        </div>
      </SettingsSection>

      <SettingsSection label="The model">
        <SettingsCard>
          <SettingsRow label="Agent" htmlFor="agent">
            <AgentFields
              catalog={catalog}
              provider={settings.provider}
              model={settings.model}
              onChange={(provider, model) => save({ provider, model, effort: "" })}
            />
          </SettingsRow>
          <SettingsRow
            label="Effort"
            htmlFor="effort"
            description="More effort is slower and uses more tokens."
            disabled={efforts.length === 0}
          >
            <EffortSelect
              id="effort"
              size="default"
              className="w-38"
              efforts={efforts}
              defaultLabel={effortDefaultLabel(catalog, null)}
              value={settings.effort}
              onChange={(effort) => save({ effort })}
            />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection label="While you approve">
        <SettingsCard>
          <SettingsRow
            label="Approve a clean rebase or merge on its own"
            htmlFor="auto-rebase"
            description={
              auto
                ? "Auto approves every turn, so this has no effect."
                : "A rebase or merge that conflicts always asks."
            }
            disabled={auto}
          >
            <Switch
              id="auto-rebase"
              checked={settings.autoApproveRebase}
              disabled={auto}
              onCheckedChange={(on) => save({ autoApproveRebase: on })}
            />
          </SettingsRow>
          <SettingsRow label="Keep the worktree when a watch stops" htmlFor="keep-worktree">
            <Switch
              id="keep-worktree"
              checked={settings.keepWorktree}
              onCheckedChange={(on) => save({ keepWorktree: on })}
            />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    </div>
  );
}

type ModeTileProps = {
  choice: ModeChoice;
  checked: boolean;
  onSelect: () => void;
};

function ModeTile({ choice, checked, onSelect }: ModeTileProps) {
  const id = `approval-mode-${choice.mode}`;
  return (
    <label
      htmlFor={id}
      className="group/tile flex min-w-0 flex-1 cursor-pointer flex-col gap-1.5 rounded-lg border bg-background p-3 transition-colors has-checked:border-primary has-checked:ring-1 has-checked:ring-primary has-focus-visible:ring-3 has-focus-visible:ring-ring/50"
    >
      <span className="flex items-center gap-2 text-sm font-medium">
        <choice.Icon className="size-4" />
        <span className="flex-1">{choice.label}</span>
        <input
          id={id}
          type="radio"
          name="approval-mode"
          value={choice.mode}
          checked={checked}
          onChange={onSelect}
          className="sr-only"
        />
        <span
          aria-hidden="true"
          className="size-4 shrink-0 rounded-full border border-input bg-background group-has-checked/tile:border-5 group-has-checked/tile:border-primary"
        />
      </span>
      <span className="text-body/snug text-muted-foreground">{choice.description}</span>
    </label>
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
