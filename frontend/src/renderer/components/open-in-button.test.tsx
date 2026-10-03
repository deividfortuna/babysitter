import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
import type { OpenTarget } from "../../shared/open-in";
import { bridge } from "@/lib/bridge";
import type { Watch } from "@/hooks/useWatches";
import { buildStoppedWatch, buildWatch } from "@test/fixtures";
import { serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import { WatchDetail } from "./watch-detail";

const WORKTREE = "/data/worktrees/octo-babysitter-12";

afterEach(() => vi.restoreAllMocks());

function installed(targets: OpenTarget[]) {
  const listed = vi.spyOn(bridge.openIn, "targets").mockResolvedValue(targets);
  const launch = vi.spyOn(bridge.openIn, "launch").mockResolvedValue({ ok: true });
  return { listed, launch };
}

function detail(id: number, onThisMachine = true) {
  return (
    <WatchDetail
      id={id}
      enabled
      onNavigate={vi.fn()}
      onStopped={vi.fn()}
      onWatchPR={vi.fn()}
      onThisMachine={onThisMachine}
    />
  );
}

function renderWatch(watch: Watch, onThisMachine = true) {
  serveApi({ watches: [watch], watchById: { [watch.id]: watch } });
  return renderWithProviders(detail(watch.id, onThisMachine));
}

async function waitForHeader() {
  await screen.findByRole("heading", { level: 1 });
}

test("opens the worktree of the watch in the first installed editor", async () => {
  const { launch } = installed(["vscode", "zed", "file-manager"]);
  const user = userEvent.setup();

  renderWatch(buildWatch({ id: 42, worktreeDir: WORKTREE }));
  await user.click(await screen.findByRole("button", { name: "Open" }));

  expect(launch).toHaveBeenCalledWith(42, "vscode");
});

test("an editor picked in the menu opens the worktree and becomes the main action", async () => {
  const { launch } = installed(["vscode", "zed", "file-manager"]);
  const user = userEvent.setup();

  renderWatch(buildWatch({ id: 42, worktreeDir: WORKTREE }));
  await user.click(await screen.findByRole("button", { name: "Open in…" }));
  await user.click(await screen.findByRole("menuitem", { name: "Zed" }));
  await user.click(screen.getByRole("button", { name: "Open" }));

  expect(launch.mock.calls).toEqual([
    [42, "zed"],
    [42, "zed"],
  ]);
  expect(window.localStorage.getItem("open_in_editor")).toBe("zed");
});

test("the menu lists each installed editor and the file manager", async () => {
  installed(["cursor", "idea", "file-manager"]);
  const user = userEvent.setup();

  renderWatch(buildWatch({ id: 42, worktreeDir: WORKTREE }));
  await user.click(await screen.findByRole("button", { name: "Open in…" }));

  const items = await screen.findAllByRole("menuitem");
  expect(items.map((item) => item.textContent)).toEqual(["Cursor", "IntelliJ IDEA", "Files"]);
});

test("an editor kept from before that is gone now falls back to the first installed one", async () => {
  window.localStorage.setItem("open_in_editor", "webstorm");
  const { launch } = installed(["zed", "file-manager"]);
  const user = userEvent.setup();

  renderWatch(buildWatch({ id: 42, worktreeDir: WORKTREE }));
  await user.click(await screen.findByRole("button", { name: "Open" }));

  expect(launch).toHaveBeenCalledWith(42, "zed");
});

test("says why the worktree did not open", async () => {
  installed(["file-manager"]).launch.mockResolvedValue({ ok: false, error: `The folder ${WORKTREE} does not exist.` });
  const user = userEvent.setup();

  renderWatch(buildWatch({ id: 42, worktreeDir: WORKTREE }));
  await user.click(await screen.findByRole("button", { name: "Open" }));

  expect(await screen.findByText(`The folder ${WORKTREE} does not exist.`)).toBeVisible();
});

test("the error of one watch does not follow the author to the next watch", async () => {
  installed(["file-manager"]).launch.mockResolvedValue({ ok: false, error: "The folder is gone." });
  const first = buildWatch({ id: 42, worktreeDir: WORKTREE, title: "First" });
  const second = buildWatch({ id: 43, worktreeDir: "/data/worktrees/octo-babysitter-13", title: "Second" });
  serveApi({ watches: [first, second], watchById: { 42: first, 43: second } });
  const user = userEvent.setup();
  const view = renderWithProviders(detail(42));
  await user.click(await screen.findByRole("button", { name: "Open" }));
  expect(await screen.findByText("The folder is gone.")).toBeVisible();

  view.rerender(detail(43));

  await screen.findByRole("heading", { level: 1, name: "Second" });
  expect(screen.queryByText("The folder is gone.")).not.toBeInTheDocument();
});

test("a self watch opens the checkout the agent worked in", async () => {
  const { launch } = installed(["file-manager"]);
  const user = userEvent.setup();

  renderWatch(buildWatch({ id: 42, provider: "self", worktreeDir: "", sourceDir: "/code/babysitter" }));
  await user.click(await screen.findByRole("button", { name: "Open" }));

  expect(launch).toHaveBeenCalledWith(42, "file-manager");
});

test("a stopped watch that kept its worktree still opens it", async () => {
  installed(["file-manager"]);

  renderWatch(buildStoppedWatch({ id: 42, worktreeDir: WORKTREE, summary: { worktreeRemoved: false } }));

  expect(await screen.findByRole("button", { name: "Open" })).toBeVisible();
});

test("a stopped watch whose worktree is gone has nothing to open, and looks for no editor", async () => {
  const { listed } = installed(["file-manager"]);

  renderWatch(buildStoppedWatch({ id: 42, worktreeDir: WORKTREE, summary: { worktreeRemoved: true } }));
  await waitForHeader();

  expect(screen.queryByRole("button", { name: "Open" })).not.toBeInTheDocument();
  expect(listed).not.toHaveBeenCalled();
});

test("a watch of a remote daemon has nothing to open, since its worktree is on another machine", async () => {
  const { listed } = installed(["vscode", "file-manager"]);

  renderWatch(buildWatch({ id: 42, worktreeDir: WORKTREE }), false);
  await waitForHeader();

  expect(screen.queryByRole("button", { name: "Open" })).not.toBeInTheDocument();
  expect(listed).not.toHaveBeenCalled();
});

test("the browser has no editor to open the worktree in", async () => {
  renderWatch(buildWatch({ id: 42, worktreeDir: WORKTREE }));
  await screen.findByRole("button", { name: "Stop watching" });

  expect(screen.queryByRole("button", { name: "Open" })).not.toBeInTheDocument();
});
