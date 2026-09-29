import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildRateLimit, buildRepo, buildSettings, buildWatch } from "@test/fixtures";
import { serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import type { RateLimit } from "@/hooks/useRateLimit";
import type { Settings } from "@/hooks/useSettings";
import { SettingsDialog } from "./settings-dialog";

type Setup = {
  settings?: Settings;
  repos?: number;
  watches?: number;
  rateLimit?: RateLimit;
};

function renderPolling({ settings = buildSettings(), repos = 0, watches = 0, rateLimit }: Setup = {}) {
  const savedSettings: Settings[] = [];
  serveApi({
    settings,
    savedSettings,
    rateLimit,
    repos: Array.from({ length: repos }, (_, i) => buildRepo({ id: i + 1, fullName: `octo/repo-${i}` })),
    watches: Array.from({ length: watches }, (_, i) => buildWatch({ id: i + 1, number: i + 1 })),
  });
  renderWithProviders(<SettingsDialog open category="polling" onOpenChange={vi.fn()} />);
  return { savedSettings, user: userEvent.setup() };
}

function tile(name: string) {
  return screen.getByRole("button", { name: new RegExp(`^${name}`) });
}

test("the tile of the stored values is pressed", async () => {
  renderPolling();

  await screen.findByRole("group", { name: "Polling speed" });
  expect(tile("Balanced")).toHaveAttribute("aria-pressed", "true");
  expect(tile("Relaxed")).toHaveAttribute("aria-pressed", "false");
  expect(tile("Custom")).toHaveAttribute("aria-pressed", "false");
  expect(screen.getByText("What a preset sets")).toBeVisible();
  expect(screen.queryByLabelText("Repository poll interval")).not.toBeInTheDocument();
});

test("a preset tile saves the four intervals at once", async () => {
  const { savedSettings, user } = renderPolling();

  await user.click(await screen.findByRole("button", { name: /^Eager/ }));

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0]).toMatchObject({
    pollIntervalSeconds: 30,
    watchIntervalSeconds: 60,
    watchMaxIntervalSeconds: 300,
    checkMaxIntervalSeconds: 300,
  });
  expect(tile("Eager")).toHaveAttribute("aria-pressed", "true");
});

test("values of no preset press Custom and show the fields", async () => {
  renderPolling({ settings: buildSettings({ pollIntervalSeconds: 45 }) });

  expect(await screen.findByLabelText("Repository poll interval")).toHaveValue(45);
  expect(tile("Custom")).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByText("What your values set")).toBeVisible();
  expect(screen.getByText("every 45s")).toBeVisible();
});

test("Custom opens the fields of the four intervals", async () => {
  const { user } = renderPolling();

  await user.click(await screen.findByRole("button", { name: /^Custom/ }));

  expect(screen.getByLabelText("Repository poll interval")).toHaveValue(60);
  expect(screen.getByLabelText("Watch poll interval", { exact: true })).toHaveValue(180);
  expect(screen.getByLabelText("Longest watch poll interval")).toHaveValue(900);
  expect(screen.getByLabelText("Longest check read interval")).toHaveValue(900);
});

test("an interval typed by hand is saved a moment after the typing stops", async () => {
  const { savedSettings, user } = renderPolling();
  await user.click(await screen.findByRole("button", { name: "Set the intervals by hand" }));

  const field = screen.getByLabelText("Watch poll interval", { exact: true });
  await user.clear(field);
  await user.type(field, "90");

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0]).toMatchObject({ watchIntervalSeconds: 90, pollIntervalSeconds: 60 });
  await waitFor(() => expect(tile("Custom")).toHaveAttribute("aria-pressed", "true"));
});

test("a watch poll interval above the longest raises the longest in the same save", async () => {
  const { savedSettings, user } = renderPolling();
  await user.click(await screen.findByRole("button", { name: "Set the intervals by hand" }));

  const field = screen.getByLabelText("Watch poll interval", { exact: true });
  await user.clear(field);
  await user.type(field, "1200");
  await user.tab();

  await waitFor(() => expect(savedSettings).toHaveLength(1));
  expect(savedSettings[0]).toMatchObject({ watchIntervalSeconds: 1200, watchMaxIntervalSeconds: 1200 });
  expect(await screen.findByText(/The longest watch poll interval moved to 20m too\./)).toBeVisible();
  await waitFor(() => expect(screen.getByLabelText("Longest watch poll interval")).toHaveValue(1200));
});

test("a longest watch poll interval below the watch poll interval is refused before it is sent", async () => {
  const { savedSettings, user } = renderPolling();
  await user.click(await screen.findByRole("button", { name: "Set the intervals by hand" }));

  const field = screen.getByLabelText("Longest watch poll interval");
  await user.clear(field);
  await user.type(field, "120");
  await user.tab();

  expect(await screen.findByText("A whole number of seconds from 180 to 86400.")).toBeVisible();
  expect(savedSettings).toHaveLength(0);
});

test("an interval out of range is refused before it is sent", async () => {
  const { savedSettings, user } = renderPolling();
  await user.click(await screen.findByRole("button", { name: "Set the intervals by hand" }));

  const field = screen.getByLabelText("Longest check read interval");
  await user.clear(field);
  await user.type(field, "5");
  await user.tab();

  expect(await screen.findByText("A whole number of seconds from 10 to 86400.")).toBeVisible();
  expect(savedSettings).toHaveLength(0);
});

test("the budget counts the repositories and the watches against the rate limit", async () => {
  renderPolling({ repos: 3, watches: 7, rateLimit: buildRateLimit({ limit: 5000 }) });

  expect(await screen.findByText("About 320 GitHub requests an hour")).toBeVisible();
  expect(screen.getByText(/of 5,000 · with 3 repositories and 7 watches/)).toBeVisible();
  expect(screen.getByText("Eager would add 9%")).toBeVisible();
  expect(screen.getByRole("meter", { name: "Share of the hourly rate limit" })).toHaveAttribute("aria-valuenow", "320");
});

test("the budget takes 5,000 while the rate limit is unknown", async () => {
  renderPolling({ repos: 1, watches: 1 });

  expect(await screen.findByText(/of 5,000 · with 1 repository and 1 watch$/)).toBeVisible();
});

test("the budget says when the values ask for more than the rate limit", async () => {
  renderPolling({ repos: 3, watches: 7, rateLimit: buildRateLimit({ limit: 100 }) });

  expect(await screen.findByText("More than the 100 GitHub requests an hour")).toBeVisible();
});

test("the fastest preset offers nothing faster", async () => {
  renderPolling({
    settings: buildSettings({
      pollIntervalSeconds: 30,
      watchIntervalSeconds: 60,
      watchMaxIntervalSeconds: 300,
      checkMaxIntervalSeconds: 300,
    }),
    repos: 1,
    watches: 1,
  });

  await waitFor(() => expect(tile("Eager")).toHaveAttribute("aria-pressed", "true"));
  expect(screen.queryByText(/would add/)).not.toBeInTheDocument();
});
