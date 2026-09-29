import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildSettings } from "@test/fixtures";
import { serveApi } from "@test/msw";
import { chooseOption, renderWithProviders } from "@test/test-utils";
import type { Settings } from "@/hooks/useSettings";
import { SettingsDialog } from "./settings-dialog";

function renderReview(settings: Settings = buildSettings()) {
  const savedSettings: Settings[] = [];
  serveApi({ settings, savedSettings });
  renderWithProviders(<SettingsDialog open category="review" onOpenChange={vi.fn()} />);
  return { savedSettings, user: userEvent.setup() };
}

function pause(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

test("the Review and merge page shows the settings of the daemon", async () => {
  renderReview(buildSettings({ approvalsRequired: 2, mergeMethod: "rebase", includeOwn: true }));

  expect(await screen.findByLabelText("Approvals before ready to merge")).toHaveValue(2);
  expect(within(screen.getByLabelText("Merge method")).getByText("Rebase")).toBeVisible();
  expect(screen.getByRole("switch", { name: "Report my own comments" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "Report the review items that already exist" })).not.toBeChecked();
});

test("a number of approvals is saved a moment after the typing stops", async () => {
  const { savedSettings, user } = renderReview();

  await user.type(await screen.findByLabelText("Approvals before ready to merge"), "2");

  expect(savedSettings).toHaveLength(0);
  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].approvalsRequired).toBe(2);
});

test("leaving the approvals field saves at once", async () => {
  const { savedSettings, user } = renderReview();

  await user.type(await screen.findByLabelText("Approvals before ready to merge"), "3");
  await user.tab();

  expect(savedSettings).toHaveLength(1);
  expect(savedSettings[0].approvalsRequired).toBe(3);
});

test("an empty approvals field asks for the rule of the base branch", async () => {
  const { savedSettings, user } = renderReview(buildSettings({ approvalsRequired: 2 }));

  await user.clear(await screen.findByLabelText("Approvals before ready to merge"));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].approvalsRequired).toBeNull();
});

test("approvals below zero are refused before they are sent", async () => {
  const { savedSettings, user } = renderReview();

  const field = await screen.findByLabelText("Approvals before ready to merge");
  await user.type(field, "-1");
  await user.tab();

  expect(await screen.findByText("The approvals take a whole number, 0 or more.")).toBeVisible();
  expect(field).toHaveAttribute("aria-invalid", "true");
  await pause(800);
  expect(savedSettings).toHaveLength(0);
});

test("the merge method is saved at once", async () => {
  const { savedSettings, user } = renderReview();

  await chooseOption(user, await screen.findByLabelText("Merge method"), "Squash");

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0].mergeMethod).toBe("squash");
});

test("the two switches of the first poll are saved at once", async () => {
  const { savedSettings, user } = renderReview();

  await user.click(await screen.findByRole("switch", { name: "Report the review items that already exist" }));
  await waitFor(() => expect(savedSettings).toHaveLength(1));
  await user.click(screen.getByRole("switch", { name: "Report my own comments" }));

  await waitFor(() => expect(savedSettings).toHaveLength(2));
  expect(savedSettings[1]).toMatchObject({ includeExisting: true, includeOwn: true });
});
