import { useMutation, useQueries, useQuery, useQueryClient, type UseQueryResult } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { proposalCodeQueryKey, watchProposalsQueryKey, watchesQueryKey } from "../lib/query-keys";

export type Proposal = components["schemas"]["HttpdProposal"];
export type ProposalDetail = components["schemas"]["HttpdProposalDetail"];
export type ProposalReply = components["schemas"]["HttpdProposalReply"];
export type ApprovalMode = components["schemas"]["HttpdWatch"]["approvalMode"];

export function useProposals(id: number | null) {
  return useQuery({
    queryKey: watchProposalsQueryKey(id ?? 0),
    enabled: id !== null,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/watches/{id}/proposals", { params: { path: { id: id ?? 0 } } });
      if (error) throw new Error(apiErrorMessage(error, "Could not load the proposals."));
      return data.proposals ?? [];
    },
  });
}

type Code = Pick<Proposal, "headSha" | "workSha">;

function proposalQuery(id: number | null, number: number | null, code?: Code, commit?: string, path?: string) {
  const query = { ...(commit ? { commit } : {}), ...(path ? { path } : {}) };
  return {
    queryKey: proposalCodeQueryKey(id ?? 0, number ?? 0, code?.headSha ?? "", code?.workSha ?? "", commit, path),
    enabled: id !== null && number !== null,
    staleTime: Infinity,
    refetchInterval: false as const,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/watches/{id}/proposals/{number}", {
        params: { path: { id: id ?? 0, number: number ?? 0 }, query: commit || path ? query : undefined },
      });
      if (error) throw new Error(apiErrorMessage(error, "Could not load the proposal."));
      return data;
    },
  };
}

export function useProposal(id: number | null, number: number | null, code?: Code, commit?: string) {
  return useQuery(proposalQuery(id, number, code, commit));
}

export type FileLoad = Pick<UseQueryResult<ProposalDetail>, "data" | "error" | "isFetching" | "refetch">;

function fileLoads(results: UseQueryResult<ProposalDetail>[]): FileLoad[] {
  return results.map(({ data, error, isFetching, refetch }) => ({ data, error, isFetching, refetch }));
}

export function useProposalFiles(
  id: number,
  number: number,
  code: Code,
  commit: string | undefined,
  paths: string[],
): FileLoad[] {
  return useQueries({
    queries: paths.map((path) => proposalQuery(id, number, code, commit, path)),
    combine: fileLoads,
  });
}

export type Approval = {
  id: number;
  number: number;
  edits?: { replyId: number; body: string }[];
  drop?: number[];
  rejectPush?: boolean;
  stopAsking?: boolean;
};

function useProposalMutation<T, R = Proposal>(fn: (vars: T) => Promise<R>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: watchesQueryKey });
    },
  });
}

export function useApproveProposal() {
  return useProposalMutation(async ({ id, number, ...body }: Approval) => {
    const { data, error } = await api().POST("/api/v1/watches/{id}/proposals/{number}/approve", {
      params: { path: { id, number } },
      body,
    });
    if (error) throw new Error(apiErrorMessage(error, "Could not approve the proposal."));
    return data;
  });
}

export function useRejectProposal() {
  return useProposalMutation(
    async ({ id, number, ...body }: { id: number; number: number; reason?: string; discard?: boolean }) => {
      const { data, error } = await api().POST("/api/v1/watches/{id}/proposals/{number}/reject", {
        params: { path: { id, number } },
        body,
      });
      if (error) throw new Error(apiErrorMessage(error, "Could not reject the proposal."));
      return data;
    },
  );
}

export function useRetryProposal() {
  return useProposalMutation(async ({ id, number }: { id: number; number: number }) => {
    const { data, error } = await api().POST("/api/v1/watches/{id}/proposals/{number}/retry", {
      params: { path: { id, number } },
    });
    if (error) throw new Error(apiErrorMessage(error, "Could not retry the proposal."));
    return data;
  });
}

export function useSetApproval() {
  return useProposalMutation(
    async ({ id, ...body }: { id: number; mode?: ApprovalMode; autoApproveRebase?: boolean; release?: boolean }) => {
      const { data, error } = await api().POST("/api/v1/watches/{id}/approval", { params: { path: { id } }, body });
      if (error) throw new Error(apiErrorMessage(error, "Could not change the approval mode."));
      return data;
    },
  );
}
