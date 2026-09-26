import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { chooseOption } from "@test/test-utils";
import { bridge } from "@/lib/bridge";
import type { UpdateSettings, UpdateStatus } from "../../shared/updates";
import { UpdatesPanel } from "./settings-updates";

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-09-26T10:05:00Z"));
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function renderPanel(status: UpdateStatus, settings: UpdateSettings = { autoDownload: true, channel: "stable" }) {
  vi.spyOn(bridge.updates, "getStatus").mockResolvedValue(status);
  vi.spyOn(bridge.updates, "getSettings").mockResolvedValue(settings);
  const setSettings = vi
    .spyOn(bridge.updates, "setSettings")
    .mockImplementation(async (patch) => ({ ...settings, ...patch }));
  render(<UpdatesPanel />);
  return { setSettings };
}

test("a build that cannot update shows its version and how to get a new one", async () => {
  renderPanel({ state: "unsupported", currentVersion: "0.1.0" });

  expect(await screen.findByText("Babysitter 0.1.0")).toBeVisible();
  expect(screen.getByText(/This build does not update itself/)).toBeVisible();
  expect(screen.queryByRole("button", { name: "Check for updates" })).toBeNull();
  expect(screen.queryByRole("switch")).toBeNull();
});

test("an app that is current says when it last checked", async () => {
  renderPanel({ state: "not-available", currentVersion: "0.2.0", checkedAt: "2026-09-26T10:00:00Z" });

  expect(await screen.findByText("Babysitter 0.2.0")).toBeVisible();
  expect(screen.getByText("You have the latest version. Checked 5 min ago.")).toBeVisible();
});

test("check for updates asks the main process and shows what it found", async () => {
  let push: (status: UpdateStatus) => void = () => undefined;
  vi.spyOn(bridge.updates, "onStatus").mockImplementation((listener) => {
    push = listener;
    return () => undefined;
  });
  renderPanel({ state: "idle", currentVersion: "0.1.0" });
  const check = vi.spyOn(bridge.updates, "check").mockImplementation(async () => {
    push({ state: "available", currentVersion: "0.1.0", version: "0.2.0" });
  });
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Check for updates" }));

  expect(check).toHaveBeenCalledOnce();
  expect(await screen.findByText("Babysitter 0.2.0 is out.")).toBeVisible();
});

test("a check cannot start while one runs", async () => {
  renderPanel({ state: "checking", currentVersion: "0.1.0" });

  expect(await screen.findByRole("button", { name: /Checking/ })).toBeDisabled();
});

test("a check that failed says why", async () => {
  renderPanel({ state: "error", currentVersion: "0.1.0", message: "No release to update from yet." });

  expect(await screen.findByRole("alert")).toHaveTextContent("No release to update from yet.");
});

test("a download that failed says why beside the version it offers", async () => {
  renderPanel({
    state: "available",
    currentVersion: "0.1.0",
    version: "0.2.0",
    message: "Could not reach GitHub. Check the connection and try again.",
  });

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Could not reach GitHub. Check the connection and try again.",
  );
  expect(screen.getByText("Babysitter 0.2.0 is out.")).toBeVisible();
});

test("the switch turns the automatic download off", async () => {
  const { setSettings } = renderPanel({ state: "idle", currentVersion: "0.1.0" });
  const user = userEvent.setup();

  const toggle = await screen.findByRole("switch", { name: "Download updates automatically" });
  expect(toggle).toBeChecked();
  await user.click(toggle);

  expect(setSettings).toHaveBeenCalledWith({ autoDownload: false });
  expect(toggle).not.toBeChecked();
});

test("the channel picks between stable and prerelease versions", async () => {
  const { setSettings } = renderPanel({ state: "idle", currentVersion: "0.1.0" });
  const user = userEvent.setup();

  const channel = await screen.findByLabelText("Channel");
  expect(channel).toHaveTextContent("Stable");
  await chooseOption(user, channel, "Prerelease");

  expect(setSettings).toHaveBeenCalledWith({ channel: "prerelease" });
});
