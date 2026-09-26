import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { buildSettings } from "@test/fixtures";
import { serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import type { Settings } from "@/hooks/useSettings";
import { SettingsDialog } from "./settings-dialog";

async function openNotifications() {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Notifications" }));
  return user;
}

test("the Notifications panel shows what the daemon holds", async () => {
  serveApi({ settings: buildSettings({ notificationsEnabled: true, notificationSound: false }) });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  await openNotifications();

  expect(await screen.findByRole("switch", { name: "Show notifications" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Play a sound" })).not.toBeChecked();
});

test("turning the notifications off is saved at once", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings(), savedSettings });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openNotifications();
  await user.click(await screen.findByRole("switch", { name: "Show notifications" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].notificationsEnabled).toBe(false);
  expect(savedSettings[0].pollIntervalSeconds).toBe(60);
});

test("the sound cannot be changed while the notifications are off", async () => {
  serveApi({ settings: buildSettings({ notificationsEnabled: false }) });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  await openNotifications();

  expect(await screen.findByRole("switch", { name: "Play a sound" })).toBeDisabled();
});

test("each kind has a switch, and a muted one is off", async () => {
  serveApi({ settings: buildSettings({ mutedNotificationKinds: ["checks"] }) });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  await openNotifications();

  expect(await screen.findByRole("switch", { name: "Review comments" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Checks" })).not.toBeChecked();
  expect(screen.getByRole("switch", { name: "Agent requests" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Watches" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Merges" })).toBeChecked();
});

test("turning a kind off mutes it and leaves the rest alone", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings({ mutedNotificationKinds: ["checks"] }), savedSettings });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openNotifications();
  await user.click(await screen.findByRole("switch", { name: "Review comments" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].mutedNotificationKinds).toEqual(["checks", "review"]);
});

test("turning a kind back on takes it off the muted list", async () => {
  const savedSettings: Settings[] = [];
  serveApi({ settings: buildSettings({ mutedNotificationKinds: ["checks", "review"] }), savedSettings });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openNotifications();
  await user.click(await screen.findByRole("switch", { name: "Checks" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].mutedNotificationKinds).toEqual(["review"]);
});

test("a muted kind this build does not know stays muted", async () => {
  const savedSettings: Settings[] = [];
  const fromANewerDaemon = ["checks", "rumour"] as Settings["mutedNotificationKinds"];
  serveApi({ settings: buildSettings({ mutedNotificationKinds: fromANewerDaemon }), savedSettings });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openNotifications();
  await user.click(await screen.findByRole("switch", { name: "Review comments" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].mutedNotificationKinds).toEqual(["checks", "rumour", "review"]);
});

test("no kind can be changed while the notifications are off", async () => {
  serveApi({ settings: buildSettings({ notificationsEnabled: false }) });

  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  await openNotifications();

  expect(await screen.findByRole("switch", { name: "Review comments" })).toBeDisabled();
});

test("a kind only a newer daemon knows has a switch of its own", async () => {
  const savedSettings: Settings[] = [];
  serveApi({
    settings: buildSettings({ mutedNotificationKinds: ["review", "deploy"] as Settings["mutedNotificationKinds"] }),
    savedSettings,
  });
  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openNotifications();

  const unknown = await screen.findByRole("switch", { name: /deploy/i });
  expect(unknown).not.toBeChecked();

  await user.click(unknown);

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].mutedNotificationKinds).toEqual(["review"]);
});

test("a kind only a newer daemon knows keeps its switch once it is on", async () => {
  const savedSettings: Settings[] = [];
  serveApi({
    settings: buildSettings({ mutedNotificationKinds: ["review", "deploy"] as Settings["mutedNotificationKinds"] }),
    savedSettings,
  });
  renderWithProviders(<SettingsDialog open onOpenChange={vi.fn()} />);
  const user = await openNotifications();

  await user.click(await screen.findByRole("switch", { name: /deploy/i }));
  await waitFor(() => expect(screen.getByRole("switch", { name: /deploy/i })).toBeChecked());

  await user.click(screen.getByRole("switch", { name: /deploy/i }));

  await waitFor(() => expect(savedSettings).toHaveLength(2));
  expect(savedSettings[1].mutedNotificationKinds).toEqual(["review", "deploy"]);
});
