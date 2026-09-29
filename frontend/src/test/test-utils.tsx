import type { ReactElement, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, type RenderOptions as TestingLibraryRenderOptions } from "@testing-library/react";
import { expect, vi } from "vite-plus/test";
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

async function openPopup(user: User, trigger: HTMLElement) {
  act(() => trigger.focus());
  await user.click(trigger);
}

export async function openMenu(user: User, trigger: HTMLElement): Promise<HTMLElement> {
  await openPopup(user, trigger);
  return screen.findByRole("menu");
}

export async function chooseOption(user: User, select: HTMLElement, name: string | RegExp) {
  expect(select).toHaveAttribute("data-slot", "select-trigger");
  await openPopup(user, select);
  await user.click(await screen.findByRole("option", { name }));
}

export async function optionLabels(user: User, select: HTMLElement) {
  await openPopup(user, select);
  const labels = (await screen.findAllByRole("option")).map((option) => option.textContent);
  await user.keyboard("{Escape}");
  return labels;
}

export type Deferred = { resolve: () => void; promise: Promise<void> };

export function deferred(): Deferred {
  let resolve = () => undefined as void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { resolve, promise };
}
