import type { Provider, ProviderEffort, ProviderModel } from "@/hooks/useProviders";
import type { ApprovalMode } from "@/hooks/useProposals";
import type { WatchOverrides } from "@/hooks/useRepos";
import type { Settings } from "@/hooks/useSettings";
import type { BranchUpdate, MergeMethod } from "@/hooks/useWatches";
import { branchUpdateLabel } from "@/lib/branch-update";
import { mergeMethodLabel } from "@/components/merge-method-select";

export type WatchDefaults = {
  provider: Provider["id"];
  model: string;
  effort: string;
  approvalMode: ApprovalMode;
  autoApproveRebase: boolean;
  approvalsRequired: number | null;
  mergeMethod: MergeMethod;
  includeExisting: boolean;
  includeOwn: boolean;
  keepWorktree: boolean;
  branchUpdate: BranchUpdate;
  updateOnGitHub: boolean;
};

export function daemonDefaults(settings: Settings): WatchDefaults {
  return {
    provider: settings.provider,
    model: settings.model,
    effort: settings.effort,
    approvalMode: settings.approvalMode,
    autoApproveRebase: settings.autoApproveRebase,
    approvalsRequired: settings.approvalsRequired ?? null,
    mergeMethod: settings.mergeMethod,
    includeExisting: settings.includeExisting,
    includeOwn: settings.includeOwn,
    keepWorktree: settings.keepWorktree,
    branchUpdate: settings.branchUpdate,
    updateOnGitHub: settings.updateOnGitHub,
  };
}

export type AgentChoice = { provider: Provider["id"]; model: string; effort: string };

type AskedAgent = { provider: Provider["id"] | ""; model: string; effort: string };

export function resolveAgent(asked: AskedAgent, layer: AgentChoice): AgentChoice {
  if (asked.provider) return { provider: asked.provider, model: asked.model, effort: asked.effort };
  if (asked.model) return { provider: layer.provider, model: asked.model, effort: asked.effort };
  return { provider: layer.provider, model: layer.model, effort: asked.effort || layer.effort };
}

export function repositoryDefaults(settings: Settings, overrides: WatchOverrides | undefined): WatchDefaults {
  const daemon = daemonDefaults(settings);
  if (!overrides) return daemon;
  return {
    ...resolveAgent(overrides, daemon),
    approvalMode: overrides.approvalMode || daemon.approvalMode,
    autoApproveRebase: overrides.autoApproveRebase ?? daemon.autoApproveRebase,
    approvalsRequired:
      overrides.approvalsRequired === undefined ? daemon.approvalsRequired : overrides.approvalsRequired,
    mergeMethod: overrides.mergeMethod || daemon.mergeMethod,
    includeExisting: overrides.includeExisting ?? daemon.includeExisting,
    includeOwn: overrides.includeOwn ?? daemon.includeOwn,
    keepWorktree: overrides.keepWorktree ?? daemon.keepWorktree,
    branchUpdate: overrides.branchUpdate || daemon.branchUpdate,
    updateOnGitHub: overrides.updateOnGitHub ?? daemon.updateOnGitHub,
  };
}

export function defaultLabel(value: string | undefined): string {
  return value ? `Default (${value})` : "Default";
}

export function mergeMethodDefaultLabel(method: MergeMethod): string {
  return mergeMethodLabel(method).toLowerCase();
}

export function branchUpdateDefaultLabel(update: BranchUpdate): string {
  return branchUpdateLabel(update).toLowerCase();
}

function modelOf(catalog: Provider[], provider: string, model: string): ProviderModel | undefined {
  return catalog.find((item) => item.id === provider)?.models?.find((item) => item.id === model);
}

export function agentLabel(catalog: Provider[], provider: string, model: string, effort = ""): string {
  const agent = catalog.find((item) => item.id === provider)?.label ?? provider;
  const modelName = model ? (modelOf(catalog, provider, model)?.label ?? model) : "";
  const named = [agent, modelName].filter(Boolean).join(" ");
  if (!effort) return named;
  return `${named} · ${effortLabel(catalog, provider, model, effort).toLowerCase()} effort`;
}

export function modelLabel(catalog: Provider[], provider: string, model: string): string {
  return modelOf(catalog, provider, model)?.label ?? (model || "provider default");
}

export function effortsOf(catalog: Provider[], provider: string, model: string): ProviderEffort[] {
  return modelOf(catalog, provider, model)?.efforts ?? [];
}

export function effortLabel(catalog: Provider[], provider: string, model: string, effort: string): string {
  if (!effort) return "model default";
  return effortsOf(catalog, provider, model).find((item) => item.id === effort)?.label ?? effort;
}

export function effortDefaultLabel(catalog: Provider[], inherited: AgentChoice | null | undefined): string {
  if (inherited === null) return "Model default";
  return defaultLabel(inherited && effortLabel(catalog, inherited.provider, inherited.model, inherited.effort));
}

export function overrideOf<T>(value: T, daemon: T | undefined): T | undefined {
  return value === daemon ? undefined : value;
}
