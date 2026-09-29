import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildSettings } from "@test/fixtures";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import type { Settings } from "@/hooks/useSettings";
import { bridge } from "@/lib/bridge";
import { testNotification } from "../../shared/notifications";
import { SettingsDialog } from "./settings-dialog";

function renderNotifications(settings: Settings = buildSettings()) {
  const savedSettings: Settings[] = [];
  serveApi({ settings, savedSettings });
  renderWithProviders(<SettingsDialog open category="notifications" onOpenChange={vi.fn()} />);
  return { savedSettings, user: userEvent.setup() };
}

test("the Notifications page shows what the daemon holds", async () => {
  renderNotifications(
    buildSettings({
      mutedNotificationKinds: ["checks"],
      silentNotificationKinds: ["review"],
      notificationsBackgroundOnly: true,
    }),
  );

  expect(await screen.findByRole("switch", { name: "Show notifications of the system" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Notify: Review comments" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Sound: Review comments" })).not.toBeChecked();
  expect(screen.getByRole("switch", { name: "Notify: Checks" })).not.toBeChecked();
  expect(screen.getByRole("switch", { name: "Sound: Agent requests" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Only while the app is in the background" })).toBeChecked();
});

test("each kind has a row with a switch to notify and a switch for the sound", async () => {
  renderNotifications();

  const table = await screen.findByRole("table", { name: "Notification kinds" });
  expect(table).toBeVisible();
  for (const label of ["Agent requests", "Review comments", "Checks", "Watches", "Merges", "Auto start"]) {
    expect(screen.getByRole("switch", { name: `Notify: ${label}` })).toBeChecked();
    expect(screen.getByRole("switch", { name: `Sound: ${label}` })).toBeChecked();
  }
});

test("turning the notifications off is saved at once", async () => {
  const { savedSettings, user } = renderNotifications();

  await user.click(await screen.findByRole("switch", { name: "Show notifications of the system" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].notificationsEnabled).toBe(false);
  expect(savedSettings[0].pollIntervalSeconds).toBe(60);
});

test("no kind can be changed while the notifications are off", async () => {
  renderNotifications(buildSettings({ notificationsEnabled: false }));

  expect(await screen.findByRole("switch", { name: "Notify: Review comments" })).toBeDisabled();
  expect(screen.getByRole("switch", { name: "Sound: Review comments" })).toBeDisabled();
  expect(screen.getByRole("switch", { name: "Only while the app is in the background" })).toBeDisabled();
});

test("the sound of a muted kind cannot be changed", async () => {
  renderNotifications(buildSettings({ mutedNotificationKinds: ["watch"] }));

  const sound = await screen.findByRole("switch", { name: "Sound: Watches" });
  expect(sound).toBeDisabled();
  expect(sound).not.toBeChecked();
  expect(screen.getByRole("switch", { name: "Sound: Merges" })).toBeEnabled();
});

test("turning a kind off mutes it and leaves the rest alone", async () => {
  const { savedSettings, user } = renderNotifications(buildSettings({ mutedNotificationKinds: ["checks"] }));

  await user.click(await screen.findByRole("switch", { name: "Notify: Review comments" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].mutedNotificationKinds).toEqual(["checks", "review"]);
});

test("turning a kind back on takes it off the muted list", async () => {
  const { savedSettings, user } = renderNotifications(buildSettings({ mutedNotificationKinds: ["checks", "review"] }));

  await user.click(await screen.findByRole("switch", { name: "Notify: Checks" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].mutedNotificationKinds).toEqual(["review"]);
});

test("turning the sound of a kind off makes it silent and leaves the rest alone", async () => {
  const { savedSettings, user } = renderNotifications(buildSettings({ silentNotificationKinds: ["auto"] }));

  await user.click(await screen.findByRole("switch", { name: "Sound: Checks" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].silentNotificationKinds).toEqual(["auto", "checks"]);
  expect(savedSettings[0].mutedNotificationKinds).toEqual([]);
});

test("turning the sound of a kind back on takes it off the silent list", async () => {
  const { savedSettings, user } = renderNotifications(buildSettings({ silentNotificationKinds: ["review", "auto"] }));

  await user.click(await screen.findByRole("switch", { name: "Sound: Review comments" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].silentNotificationKinds).toEqual(["auto"]);
});

test("the background only switch is saved at once", async () => {
  const { savedSettings, user } = renderNotifications();

  await user.click(await screen.findByRole("switch", { name: "Only while the app is in the background" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].notificationsBackgroundOnly).toBe(true);
});

test("a muted kind this build does not know stays muted", async () => {
  const fromANewerDaemon = ["checks", "rumour"] as Settings["mutedNotificationKinds"];
  const { savedSettings, user } = renderNotifications(buildSettings({ mutedNotificationKinds: fromANewerDaemon }));

  await user.click(await screen.findByRole("switch", { name: "Notify: Review comments" }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].mutedNotificationKinds).toEqual(["checks", "rumour", "review"]);
});

test("a kind only a newer daemon knows has a row of its own", async () => {
  const { savedSettings, user } = renderNotifications(
    buildSettings({ mutedNotificationKinds: ["review", "deploy"] as Settings["mutedNotificationKinds"] }),
  );

  const unknown = await screen.findByRole("switch", { name: "Notify: deploy" });
  expect(unknown).not.toBeChecked();

  await user.click(unknown);

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].mutedNotificationKinds).toEqual(["review"]);
});

test("a kind only a newer daemon knows keeps its row once it is on", async () => {
  const { savedSettings, user } = renderNotifications(
    buildSettings({ mutedNotificationKinds: ["review", "deploy"] as Settings["mutedNotificationKinds"] }),
  );

  await user.click(await screen.findByRole("switch", { name: "Notify: deploy" }));
  await waitFor(() => expect(screen.getByRole("switch", { name: "Notify: deploy" })).toBeChecked());

  await user.click(screen.getByRole("switch", { name: "Notify: deploy" }));

  await waitFor(() => expect(savedSettings).toHaveLength(2));
  expect(savedSettings[1].mutedNotificationKinds).toEqual(["review", "deploy"]);
});

test("a silent kind only a newer daemon knows has a row of its own", async () => {
  renderNotifications(
    buildSettings({ silentNotificationKinds: ["review", "deploy"] as Settings["silentNotificationKinds"] }),
  );

  expect(await screen.findByRole("switch", { name: "Sound: deploy" })).not.toBeChecked();
  expect(screen.getByRole("switch", { name: "Notify: deploy" })).toBeChecked();
});

test("Send a test shows a notification of the system", async () => {
  vi.spyOn(bridge.notifications, "supported").mockResolvedValue(true);
  const show = vi.spyOn(bridge.notifications, "show").mockResolvedValue(undefined);
  const { user } = renderNotifications();

  const send = await screen.findByRole("button", { name: "Send a test" });
  await waitFor(() => expect(send).toBeEnabled());
  await user.click(send);

  expect(show).toHaveBeenCalledWith(testNotification());
});

test("Send a test waits for a window that can show notifications", async () => {
  vi.spyOn(bridge.notifications, "supported").mockResolvedValue(false);
  renderNotifications();

  expect(await screen.findByRole("button", { name: "Send a test" })).toBeDisabled();
});

test("Send a test is off while the notifications are off", async () => {
  const supported = vi.spyOn(bridge.notifications, "supported").mockResolvedValue(true);
  renderNotifications(buildSettings({ notificationsEnabled: false }));

  const send = await screen.findByRole("button", { name: "Send a test" });
  await waitFor(() => expect(supported).toHaveBeenCalled());
  expect(send).toBeDisabled();
});

test("a save the daemon refuses shows what the daemon answered", async () => {
  const { user } = renderNotifications();
  server.use(
    http.put(apiUrl("/api/v1/settings"), () =>
      HttpResponse.json({ error: { code: "bad_request", message: "the store is read only" } }, { status: 400 }),
    ),
  );

  const toggle = await screen.findByRole("switch", { name: "Notify: Merges" });
  await user.click(toggle);

  expect(await screen.findByText("the store is read only")).toBeVisible();
  await waitFor(() => expect(toggle).toBeChecked());
});
