import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildRateLimit } from "@test/fixtures";
import { serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import { RateLimitCard } from "./rate-limit-card";
import { AppearancePanel } from "./settings-appearance";

function renderPanelAndCard() {
  serveApi({ rateLimit: buildRateLimit({ state: "ok", remaining: 4212 }) });
  renderWithProviders(
    <>
      <AppearancePanel />
      <RateLimitCard enabled onPollLessOften={vi.fn()} />
    </>,
  );
}

test("the Appearance page holds the theme and the rate limit of the sidebar", () => {
  renderWithProviders(<AppearancePanel />);

  expect(screen.getByRole("group", { name: "Theme" })).toBeVisible();
  expect(screen.getByRole("switch", { name: "Always show the GitHub rate limit" })).not.toBeChecked();
});

test("always showing the rate limit puts the card in the sidebar at once", async () => {
  const user = userEvent.setup();
  renderPanelAndCard();

  const toggle = screen.getByRole("switch", { name: "Always show the GitHub rate limit" });
  await user.click(toggle);

  expect(toggle).toBeChecked();
  expect(await screen.findByRole("region", { name: "GitHub rate limit" })).toBeVisible();
});

test("turning the setting off hides a budget that is not relevant again", async () => {
  window.localStorage.setItem("always_show_rate_limit", "true");
  const user = userEvent.setup();
  renderPanelAndCard();

  await screen.findByRole("region", { name: "GitHub rate limit" });
  await user.click(screen.getByRole("switch", { name: "Always show the GitHub rate limit" }));

  expect(screen.queryByRole("region", { name: "GitHub rate limit" })).not.toBeInTheDocument();
});

test("keeps the choice for the next start of the app", async () => {
  const user = userEvent.setup();
  renderWithProviders(<AppearancePanel />);

  await user.click(screen.getByRole("switch", { name: "Always show the GitHub rate limit" }));

  expect(window.localStorage.getItem("always_show_rate_limit")).toBe("true");
});
