import type { Provider } from "@/hooks/useProviders";
import type { ApprovalMode } from "@/hooks/useProposals";
import type { WatchOverrides } from "@/hooks/useRepos";
import type { Settings } from "@/hooks/useSettings";
import type { MergeMethod } from "@/hooks/useWatches";
import { mergeMethodLabel } from "@/components/merge-method-select";

export type WatchDefaults = {
  provider: Provider["id"];
  model: string;
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
  const agent = overrides.provider ? { provider: overrides.provider, model: overrides.model } : daemon;
  return {
    provider: agent.provider,
    model: agent.model,
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

export function agentLabel(catalog: Provider[], provider: string, model: string): string {
  const entry = catalog.find((item) => item.id === provider);
  const agent = entry?.label ?? provider;
  if (!model) return agent;
  const modelName = entry?.models?.find((item) => item.id === model)?.label ?? model;
  return `${agent} ${modelName}`;
}

export function modelLabel(catalog: Provider[], provider: string, model: string): string {
  const models = catalog.find((item) => item.id === provider)?.models ?? [];
  return models.find((item) => item.id === model)?.label ?? (model || "provider default");
}

export function overrideOf<T>(value: T, daemon: T | undefined): T | undefined {
  return value === daemon ? undefined : value;
}
