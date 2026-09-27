import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { buildRateLimit } from "@test/fixtures";
import { serveApi } from "@test/msw";
import { createQueryClientForTests, renderWithProviders } from "@test/test-utils";
import type { RateLimit } from "@/hooks/useRateLimit";
import { rateLimitQueryKey } from "@/lib/query-keys";
import { RateLimitCard } from "./rate-limit-card";

const now = new Date("2026-09-24T12:00:00Z");

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(now);
});

afterEach(() => {
  vi.useRealTimers();
});

function renderCard(rateLimit: RateLimit, onPollLessOften = vi.fn()) {
  serveApi({ rateLimit });
  renderWithProviders(<RateLimitCard enabled onPollLessOften={onPollLessOften} />);
  return onPollLessOften;
}

async function findCard() {
  return screen.findByRole("region", { name: "GitHub rate limit" });
}

test("shows the budget that is left and when it resets once more than half is used", async () => {
  renderCard(buildRateLimit({ state: "ok", remaining: 2079, resetAt: "2026-09-24T12:38:00Z" }));

  const card = await findCard();

  expect(within(card).getByText("GitHub API")).toBeVisible();
  expect(within(card).getByText("resets in 38m")).toBeVisible();
  expect(within(card).getByText("2,079 of 5,000 left")).toBeVisible();
  expect(within(card).getByRole("progressbar", { name: "GitHub requests used this hour" })).toHaveAttribute(
    "aria-valuenow",
    "58.4",
  );
  expect(within(card).queryByRole("button")).not.toBeInTheDocument();
});

test("shows nothing while half of the budget or less is used", async () => {
  serveApi({ rateLimit: buildRateLimit({ state: "ok", remaining: 2500 }) });
  const queryClient = createQueryClientForTests();
  renderWithProviders(<RateLimitCard enabled onPollLessOften={vi.fn()} />, { queryClient });

  await vi.waitFor(() => expect(queryClient.getQueryState(rateLimitQueryKey)?.status).toBe("success"));

  expect(screen.queryByRole("region", { name: "GitHub rate limit" })).not.toBeInTheDocument();
});

test("shows the budget at any use when the developer setting always shows it", async () => {
  window.localStorage.setItem("always_show_rate_limit", "true");
  renderCard(buildRateLimit({ state: "ok", remaining: 4212 }));

  const card = await findCard();

  expect(within(card).getByText("4,212 of 5,000 left")).toBeVisible();
});

test("says the budget is nearly used and offers to poll less often", async () => {
  const onPollLessOften = renderCard(buildRateLimit({ state: "low", remaining: 312, resetAt: "2026-09-24T12:21:00Z" }));
  const user = userEvent.setup();

  const card = await findCard();
  expect(within(card).getByText("Rate limit nearly used")).toBeVisible();
  expect(within(card).getByText("resets in 21m")).toBeVisible();
  expect(within(card).getByText("312 of 5,000 left")).toBeVisible();
  expect(
    within(card).getByText("Polling pauses before the limit runs out, then goes on by itself at the reset."),
  ).toBeVisible();

  await user.click(within(card).getByRole("button", { name: "Poll less often" }));

  expect(onPollLessOften).toHaveBeenCalled();
});

test("says the polling is paused until the reset", async () => {
  renderCard(buildRateLimit({ state: "paused", remaining: 78, resetAt: "2026-09-24T12:12:00Z" }));

  const card = await findCard();

  expect(within(card).getByText("Polling paused")).toBeVisible();
  expect(within(card).getByText("resets in 12m")).toBeVisible();
  expect(within(card).getByText("78 of 5,000 left")).toBeVisible();
  expect(
    within(card).getByText(
      "The GitHub rate limit is nearly used. Every watch picks up where it left off at the reset, and nothing is lost.",
    ),
  ).toBeVisible();
});

test("says GitHub asked to slow down and when the polls go on", async () => {
  renderCard(
    buildRateLimit({
      state: "slowed",
      remaining: 2880,
      resetAt: "2026-09-24T12:40:00Z",
      retryAt: "2026-09-24T12:00:50Z",
    }),
  );

  const card = await findCard();

  expect(within(card).getByText("GitHub asked to slow down")).toBeVisible();
  expect(within(card).getByText("retry in 1m")).toBeVisible();
  expect(within(card).getByText("2,880 of 5,000 left")).toBeVisible();
  expect(
    within(card).getByText(
      "Too many requests in a short time. The daemon waits the minute GitHub asked for and goes on.",
    ),
  ).toBeVisible();
});

test("leaves the reset out once the window passed", async () => {
  window.localStorage.setItem("always_show_rate_limit", "true");
  renderCard(buildRateLimit({ state: "ok", remaining: 5000, resetAt: undefined }));

  const card = await findCard();

  expect(within(card).getByText("5,000 of 5,000 left")).toBeVisible();
  expect(within(card).queryByText(/resets in/)).not.toBeInTheDocument();
});

test("dismiss keeps the budget and puts the note away", async () => {
  renderCard(buildRateLimit({ state: "low", remaining: 312, resetAt: "2026-09-24T12:21:00Z" }));
  const user = userEvent.setup();

  const card = await findCard();
  await user.click(within(card).getByRole("button", { name: "Dismiss" }));

  expect(within(card).getByText("312 of 5,000 left")).toBeVisible();
  expect(within(card).queryByText(/Polling pauses before the limit runs out/)).not.toBeInTheDocument();
  expect(within(card).queryByRole("button")).not.toBeInTheDocument();
});

test("shows nothing before GitHub answered a call", async () => {
  serveApi({ rateLimit: buildRateLimit({ state: "unknown", limit: 0, remaining: 0 }) });
  const queryClient = createQueryClientForTests();
  renderWithProviders(<RateLimitCard enabled onPollLessOften={vi.fn()} />, { queryClient });

  await vi.waitFor(() => expect(queryClient.getQueryState(rateLimitQueryKey)?.status).toBe("success"));

  expect(screen.queryByRole("region", { name: "GitHub rate limit" })).not.toBeInTheDocument();
});
