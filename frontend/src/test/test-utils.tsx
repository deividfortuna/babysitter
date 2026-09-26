import type { ReactElement, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, type RenderOptions as TestingLibraryRenderOptions } from "@testing-library/react";
import { expect, vi } from "vitest";
import { SidebarProvider } from "@/components/ui/sidebar";
import { ThemeProvider } from "@/hooks/use-theme";

type MediaListener = (event: MediaQueryListEvent) => void;

export function stubMatchMedia(matches: boolean) {
  let current = matches;
  const listeners = new Set<MediaListener>();

  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      get matches() {
        return current;
      },
      addEventListener: (_: string, listener: MediaListener) => listeners.add(listener),
      removeEventListener: (_: string, listener: MediaListener) => listeners.delete(listener),
    })),
  );

  return {
    set(next: boolean) {
      current = next;
      for (const listener of listeners) listener({ matches: next } as MediaQueryListEvent);
    },
    get listenerCount() {
      return listeners.size;
    },
  };
}

export function createQueryClientForTests() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: 0,
        retry: false,
        refetchOnReconnect: false,
        refetchOnWindowFocus: false,
        staleTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  });
}

type RenderOptions = {
  queryClient?: QueryClient;
  withSidebar?: boolean;
} & Omit<TestingLibraryRenderOptions, "wrapper">;

export function renderWithProviders(
  ui: ReactElement,
  { queryClient = createQueryClientForTests(), withSidebar = false, ...options }: RenderOptions = {},
) {
  function Wrapper({ children }: { children: ReactNode }) {
    const content = withSidebar ? <SidebarProvider>{children}</SidebarProvider> : children;
    return (
      <ThemeProvider>
        <QueryClientProvider client={queryClient}>{content}</QueryClientProvider>
      </ThemeProvider>
    );
  }

  return { queryClient, ...render(ui, { wrapper: Wrapper, ...options }) };
}

export function expectViewTitle(name: string) {
  expect(screen.getByRole("banner")).toContainElement(screen.getByRole("heading", { level: 1, name }));
}

type User = { click: (element: Element) => Promise<void>; keyboard: (text: string) => Promise<void> };

async function openSelect(user: User, select: HTMLElement) {
  select.focus();
  await user.click(select);
}

export async function chooseOption(user: User, select: HTMLElement, name: string | RegExp) {
  expect(select).toHaveAttribute("data-slot", "select-trigger");
  await openSelect(user, select);
  await user.click(await screen.findByRole("option", { name }));
}

export async function optionLabels(user: User, select: HTMLElement) {
  await openSelect(user, select);
  const labels = (await screen.findAllByRole("option")).map((option) => option.textContent);
  await user.keyboard("{Escape}");
  return labels;
}
