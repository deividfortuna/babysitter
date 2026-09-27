import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { buildRateLimit } from "@test/fixtures";
import { serveApi } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import { RateLimitCard } from "./rate-limit-card";
import { DeveloperPanel } from "./settings-developer";

function renderPanelAndCard() {
  serveApi({ rateLimit: buildRateLimit({ state: "ok", remaining: 4212 }) });
  renderWithProviders(
    <>
      <DeveloperPanel />
      <RateLimitCard enabled onPollLessOften={vi.fn()} />
    </>,
  );
}

test("always showing the rate limit puts the card in the sidebar at once", async () => {
  const user = userEvent.setup();
  renderPanelAndCard();

  const toggle = screen.getByRole("switch", { name: "Always show the GitHub rate limit" });
  expect(toggle).not.toBeChecked();

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
  renderWithProviders(<DeveloperPanel />);

  await user.click(screen.getByRole("switch", { name: "Always show the GitHub rate limit" }));

  expect(window.localStorage.getItem("always_show_rate_limit")).toBe("true");
});
