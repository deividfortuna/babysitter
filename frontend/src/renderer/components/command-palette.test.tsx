import { useState } from "react";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildPullRequest, buildRepo, buildStoppedWatch, buildWatch } from "@test/fixtures";
import { serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import type { HistoryControls } from "@/hooks/use-view-history";
import { CommandPalette } from "./command-palette";

const noHistory: HistoryControls = { canGoBack: false, canGoForward: false, onBack: vi.fn(), onForward: vi.fn() };

type PaletteOptions = { enabled?: boolean; history?: HistoryControls };

function renderPalette({ enabled = true, history = noHistory }: PaletteOptions = {}) {
  const props = {
    onNavigate: vi.fn(),
    onWatchPR: vi.fn(),
    onWatchPull: vi.fn(),
    onAddRepo: vi.fn(),
    onPair: vi.fn(),
    onOpenSettings: vi.fn(),
  };
  const onOpenChange = vi.fn();

  function Harness() {
    const [open, setOpen] = useState(true);
    return (
      <CommandPalette
        open={open}
        onOpenChange={(next) => {
          onOpenChange(next);
          setOpen(next);
        }}
        enabled={enabled}
        history={history}
        {...props}
      />
    );
  }

  renderWithProviders(<Harness />, { withSidebar: true });
  return { ...props, onOpenChange, user: userEvent.setup() };
}

test("goes to a watch that matches the typed title and closes", async () => {
  serveApi({
    watches: [buildWatch({ id: 7, title: "Fix the flaky poll" }), buildStoppedWatch({ id: 8, title: "Old work" })],
  });
  const { user, onNavigate, onOpenChange } = renderPalette();
  await screen.findByRole("option", { name: /Fix the flaky poll/ });

  await user.keyboard("flaky{Enter}");

  expect(onNavigate).toHaveBeenCalledExactlyOnceWith({ kind: "watch", id: 7 });
  expect(onOpenChange).toHaveBeenCalledWith(false);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

test("lists only the active watches", async () => {
  serveApi({
    watches: [buildStoppedWatch({ id: 8, title: "Old work" }), buildWatch({ id: 7, title: "Fix the flaky poll" })],
  });
  renderPalette();
  await screen.findByRole("option", { name: /Fix the flaky poll/ });

  expect(screen.queryByRole("option", { name: /Old work/ })).not.toBeInTheDocument();
});

test("a search does not match the id of a watch", async () => {
  serveApi({ watches: [buildWatch({ id: 12, number: 340, title: "Alpha" })] });
  const { user } = renderPalette();
  await screen.findByRole("option", { name: /Alpha/ });

  await user.keyboard("12");

  expect(screen.queryByRole("option", { name: /Alpha/ })).not.toBeInTheDocument();
});

test("a search does not match the author of a watch, which its row does not show", async () => {
  serveApi({ watches: [buildWatch({ title: "Alpha", author: "hubot" })] });
  const { user } = renderPalette();
  await screen.findByRole("option", { name: /Alpha/ });

  await user.keyboard("hubot");

  expect(screen.queryByRole("option", { name: /Alpha/ })).not.toBeInTheDocument();
});

test("goes to a repository", async () => {
  serveApi({ repos: [buildRepo({ id: 3, fullName: "octo/dashboard" })] });
  const { user, onNavigate } = renderPalette();

  await user.click(await screen.findByRole("option", { name: "octo/dashboard" }));

  expect(onNavigate).toHaveBeenCalledExactlyOnceWith({ kind: "repo", name: "octo/dashboard" });
});

test("offers to watch an open pull request that no active watch has", async () => {
  const unwatched = buildPullRequest({ number: 30, title: "Bump the linter" });
  serveApi({
    watches: [buildWatch({ number: 12, title: "Add notifications" })],
    pullRequests: [buildPullRequest({ number: 12, title: "Add notifications" }), unwatched],
  });
  const { user, onWatchPull } = renderPalette();

  const group = await screen.findByRole("group", { name: "Open pull requests" });
  expect(group).not.toHaveTextContent("Add notifications");

  await user.click(screen.getByRole("option", { name: /Bump the linter/ }));

  expect(onWatchPull).toHaveBeenCalledExactlyOnceWith(unwatched);
});

test("the prefix switches from places to actions and back", async () => {
  serveApi({ repos: [buildRepo({ fullName: "octo/dashboard" })] });
  const { user } = renderPalette();
  await screen.findByRole("option", { name: "octo/dashboard" });

  await user.keyboard(">");

  expect(screen.queryByRole("option", { name: "octo/dashboard" })).not.toBeInTheDocument();
  expect(screen.getByRole("option", { name: "Watch a pull request" })).toBeInTheDocument();

  await user.keyboard("{Backspace}");

  expect(screen.getByRole("option", { name: "octo/dashboard" })).toBeInTheDocument();
  expect(screen.queryByRole("option", { name: "Watch a pull request" })).not.toBeInTheDocument();
});

test("lists the actions in their order after switching from places", async () => {
  serveApi({ repos: [buildRepo({ fullName: "octo/dashboard" })] });
  const { user } = renderPalette();
  await screen.findByRole("option", { name: "octo/dashboard" });

  await user.keyboard(">");

  const actions = within(screen.getByRole("group", { name: "Actions" })).getAllByRole("option");
  expect(actions.map((option) => option.textContent)).toEqual([
    "Watch a pull request",
    "Add a repository",
    "Sync now",
    "Connect to a remote daemon",
    "Toggle sidebarCtrl+B",
  ]);
});

test("lists a history move in its place when it can go", async () => {
  serveApi();
  const { user } = renderPalette({ history: { ...noHistory, canGoBack: true } });

  await user.keyboard(">");

  const actions = within(screen.getByRole("group", { name: "Actions" })).getAllByRole("option");
  expect(actions.map((option) => option.textContent)).toEqual([
    "Watch a pull request",
    "Add a repository",
    "Sync now",
    "Connect to a remote daemon",
    "Go backAlt+←",
    "Toggle sidebarCtrl+B",
  ]);
});

test("puts the best match first", async () => {
  serveApi();
  const { user } = renderPalette();

  await user.keyboard(">sync");

  expect(screen.getAllByRole("option")[0]).toHaveTextContent("Sync now");
});

test("says nothing matches", async () => {
  serveApi();
  const { user } = renderPalette();

  await user.keyboard("zzzz");

  expect(await screen.findByText("Nothing matches. Type > for actions.")).toBeInTheDocument();
  expect(screen.queryByRole("option")).not.toBeInTheDocument();
});

test("shows the state of a watch before its repository", async () => {
  serveApi({ watches: [buildWatch({ title: "Mine now", repo: "octo/a-very-long-repository-name" })] });
  renderPalette();

  expect(await screen.findByRole("option", { name: /Mine now/ })).toHaveTextContent(
    "watching · octo/a-very-long-repository-name",
  );
});

test("says a watch that the user took over is with them", async () => {
  serveApi({ watches: [buildWatch({ title: "Mine now", takenOverAt: "2026-10-01T00:00:00Z" })] });
  renderPalette();

  expect(await screen.findByRole("option", { name: /Mine now/ })).toHaveTextContent("with you");
});

test("opens a settings page from an action", async () => {
  serveApi();
  const { user, onOpenSettings } = renderPalette();

  await user.keyboard(">github{Enter}");

  expect(onOpenSettings).toHaveBeenCalledExactlyOnceWith("github");
});

test("hides the actions that need the daemon while it is down", async () => {
  serveApi();
  const { user } = renderPalette({ enabled: false });

  await user.keyboard(">");

  expect(screen.queryByRole("option", { name: "Watch a pull request" })).not.toBeInTheDocument();
  expect(screen.queryByRole("option", { name: "Add a repository" })).not.toBeInTheDocument();
  expect(screen.queryByRole("option", { name: "Sync now" })).not.toBeInTheDocument();
  expect(screen.getByRole("option", { name: "Connect to a remote daemon" })).toBeInTheDocument();
});

test("hides the history moves that have nowhere to go", async () => {
  serveApi();
  const { user } = renderPalette();

  await user.keyboard(">");

  expect(screen.queryByRole("option", { name: /Go back/ })).not.toBeInTheDocument();
  expect(screen.queryByRole("option", { name: /Go forward/ })).not.toBeInTheDocument();
});
