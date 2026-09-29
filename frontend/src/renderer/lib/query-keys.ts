export const reposQueryKey = ["repos"] as const;

export function repoConfigQueryKey(id: number) {
  return ["repos", id, "config"] as const;
}

export function repoQueueQueryKey(id: number) {
  return ["repos", id, "queue"] as const;
}
export const pullsQueryKey = ["prs"] as const;

export function pullListQueryKey(state: "open" | "all") {
  return ["prs", state] as const;
}
export const watchesQueryKey = ["watches"] as const;

export function watchListQueryKey(status: "active" | "all") {
  return ["watches", "list", status] as const;
}

export function watchActivityQueryKey(id: number) {
  return ["watches", id, "activity"] as const;
}

export function watchOutputQueryKey(id: number) {
  return ["watches", id, "output"] as const;
}

export function watchProposalsQueryKey(id: number) {
  return ["watches", id, "proposals"] as const;
}

export function proposalCodeQueryKey(
  id: number,
  number: number,
  headSha: string,
  workSha: string,
  commit = "",
  path = "",
) {
  return ["proposal-code", id, number, headSha, workSha, commit, path] as const;
}

export const providersQueryKey = ["providers"] as const;

export const settingsQueryKey = ["settings"] as const;
export const settingsMutationKey = ["settings"] as const;

export const notificationsQueryKey = ["notifications"] as const;

export const viewerQueryKey = ["viewer"] as const;

export const rateLimitQueryKey = ["ratelimit"] as const;

export const logLevelQueryKey = ["logs", "level"] as const;
