import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { authQueryKey, viewerQueryKey } from "../lib/query-keys";

export type Auth = components["schemas"]["HttpdAuth"];
export type SignInPrompt = components["schemas"]["HttpdSignInPrompt"];
export type AuthInstallation = components["schemas"]["HttpdAuthInstallation"];
export type TokenOrigin = Auth["origin"];

export function useAuth() {
  return useQuery({
    queryKey: authQueryKey,
    staleTime: Infinity,
    retry: false,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/auth");
      if (error) throw new Error(apiErrorMessage(error, "Could not read the GitHub access of the daemon."));
      return data;
    },
  });
}

function useAuthChange<T>(mutationFn: () => Promise<T>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: authQueryKey });
      void queryClient.invalidateQueries({ queryKey: viewerQueryKey });
    },
  });
}

export function useStartSignIn() {
  return useAuthChange(async () => {
    const { data, error } = await api().POST("/api/v1/auth/signin");
    if (error) throw new Error(apiErrorMessage(error, "Could not start the sign in."));
    return data;
  });
}

export function useCancelSignIn() {
  return useAuthChange(async () => {
    const { error } = await api().DELETE("/api/v1/auth/signin");
    if (error) throw new Error(apiErrorMessage(error, "Could not cancel the sign in."));
  });
}

export function useSignOut() {
  return useAuthChange(async () => {
    const { error } = await api().POST("/api/v1/auth/signout");
    if (error) throw new Error(apiErrorMessage(error, "Could not sign out."));
  });
}
