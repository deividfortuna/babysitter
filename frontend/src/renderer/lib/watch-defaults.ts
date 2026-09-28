import type { Provider, ProviderEffort } from "@/hooks/useProviders";
import type { ApprovalMode } from "@/hooks/useProposals";
import type { WatchOverrides } from "@/hooks/useRepos";
import type { Settings } from "@/hooks/useSettings";
import type { MergeMethod } from "@/hooks/useWatches";
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
  };
}

export function repositoryDefaults(settings: Settings, overrides: WatchOverrides | undefined): WatchDefaults {
  const daemon = daemonDefaults(settings);
  if (!overrides) return daemon;
  const agent = overrides.provider
    ? { provider: overrides.provider, model: overrides.model, effort: overrides.effort }
    : daemon;
  return {
    provider: agent.provider,
    model: agent.model,
    effort: agent.effort,
    approvalMode: overrides.approvalMode || daemon.approvalMode,
    autoApproveRebase: overrides.autoApproveRebase ?? daemon.autoApproveRebase,
    approvalsRequired:
      overrides.approvalsRequired === undefined ? daemon.approvalsRequired : overrides.approvalsRequired,
    mergeMethod: overrides.mergeMethod || daemon.mergeMethod,
    includeExisting: overrides.includeExisting ?? daemon.includeExisting,
    includeOwn: overrides.includeOwn ?? daemon.includeOwn,
    keepWorktree: overrides.keepWorktree ?? daemon.keepWorktree,
  };
}

export function defaultLabel(value: string | undefined): string {
  return value ? `Default (${value})` : "Default";
}

export function mergeMethodDefaultLabel(method: MergeMethod): string {
  return mergeMethodLabel(method).toLowerCase();
}

export function agentLabel(catalog: Provider[], provider: string, model: string, effort = ""): string {
  const entry = catalog.find((item) => item.id === provider);
  const agent = entry?.label ?? provider;
  const modelName = model ? (entry?.models?.find((item) => item.id === model)?.label ?? model) : "";
  const named = [agent, modelName].filter(Boolean).join(" ");
  return effort ? `${named} · ${effortPhrase(catalog, provider, model, effort)}` : named;
}

export function modelLabel(catalog: Provider[], provider: string, model: string): string {
  const models = catalog.find((item) => item.id === provider)?.models ?? [];
  return models.find((item) => item.id === model)?.label ?? (model || "provider default");
}

export function effortsOf(catalog: Provider[], provider: string, model: string): ProviderEffort[] {
  const models = catalog.find((item) => item.id === provider)?.models ?? [];
  return models.find((item) => item.id === model)?.efforts ?? [];
}

export function effortLabel(catalog: Provider[], provider: string, model: string, effort: string): string {
  if (!effort) return "model default";
  return effortsOf(catalog, provider, model).find((item) => item.id === effort)?.label ?? effort;
}

export function effortPhrase(catalog: Provider[], provider: string, model: string, effort: string): string {
  return `${effortLabel(catalog, provider, model, effort).toLowerCase()} effort`;
}

export function overrideOf<T>(value: T, daemon: T | undefined): T | undefined {
  return value === daemon ? undefined : value;
}
