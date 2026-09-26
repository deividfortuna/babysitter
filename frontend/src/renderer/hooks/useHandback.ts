import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { watchesQueryKey } from "../lib/query-keys";

export type WorkCommit = components["schemas"]["HttpdWorkCommit"];
type HandbackRefusal = components["schemas"]["HttpdHandbackRefusal"];

export class HandbackError extends Error {
  readonly code: string;
  readonly commits: WorkCommit[];
  readonly files: string[];

  constructor(refusal: HandbackRefusal) {
    super(refusal.error.message);
    this.code = refusal.error.code;
    this.commits = refusal.commits ?? [];
    this.files = refusal.files ?? [];
  }

  get asksToConfirm(): boolean {
    return this.code === "unconfirmed_work";
  }

  get authorPid(): string | null {
    if (this.code !== "author_running") return null;
    return /pid (\d+)/.exec(this.message)?.[1] ?? "";
  }
}

export function useHandback() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, confirm }: { id: number; confirm?: boolean }) => {
      const { data, error } = await api().POST("/api/v1/watches/{id}/handback", {
        params: { path: { id } },
        body: confirm ? { confirm } : {},
      });
      if (error && "error" in error && error.error.code) throw new HandbackError(error as HandbackRefusal);
      if (error) throw new Error(apiErrorMessage(error, "Could not hand the session back."));
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: watchesQueryKey });
    },
  });
}
