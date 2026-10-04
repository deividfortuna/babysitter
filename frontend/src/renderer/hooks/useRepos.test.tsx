import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { expect, test } from "vite-plus/test";
import { serveApi } from "@test/msw";
import { createQueryClientForTests } from "@test/test-utils";
import { useRemoveRepo } from "./useRepos";

function testWrapper() {
  const queryClient = createQueryClientForTests();
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

test("removes a repository on an empty daemon response", async () => {
  serveApi();
  const { result } = renderHook(() => useRemoveRepo(), { wrapper: testWrapper() });

  result.current.mutate(1);

  await waitFor(() => expect(result.current.isSuccess).toBe(true));
});
