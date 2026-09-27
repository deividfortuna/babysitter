import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { bridge } from "@/lib/bridge";
import type { UpdateStatus } from "../../shared/updates";
import { UpdateCard } from "./update-card";

afterEach(() => {
  vi.restoreAllMocks();
});

function renderCard(status: UpdateStatus) {
  vi.spyOn(bridge.updates, "getStatus").mockResolvedValue(status);
  render(<UpdateCard />);
}

async function findCard() {
  return screen.findByRole("region", { name: "App update" });
}

test.each<UpdateStatus["state"]>(["unsupported", "idle", "checking", "not-available", "error"])(
  "shows nothing while the update is %s",
  async (state) => {
    const getStatus = vi
      .spyOn(bridge.updates, "getStatus")
      .mockResolvedValue({ state, currentVersion: "0.1.0", message: "offline" });
    render(<UpdateCard />);

    await vi.waitFor(() => expect(getStatus).toHaveBeenCalled());
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(screen.queryByRole("region", { name: "App update" })).toBeNull();
  },
);

test("a new version waiting for the user offers the download and the release notes", async () => {
  const download = vi.spyOn(bridge.updates, "download").mockResolvedValue(undefined);
  renderCard({ state: "available", currentVersion: "0.1.0", version: "0.2.0" });

  const card = await findCard();

  expect(within(card).getByText("Babysitter 0.2.0 is out")).toBeVisible();
  expect(within(card).getByRole("link", { name: "What's new" })).toHaveAttribute(
    "href",
    "https://github.com/deividfortuna/babysitter/releases/tag/v0.2.0",
  );
  await userEvent.click(within(card).getByRole("button", { name: "Download" }));
  expect(download).toHaveBeenCalledOnce();
});

test("a download that failed says why and offers the download again", async () => {
  const download = vi.spyOn(bridge.updates, "download").mockResolvedValue(undefined);
  renderCard({
    state: "available",
    currentVersion: "0.1.0",
    version: "0.2.0",
    message: "Could not reach GitHub. Check the connection and try again.",
  });

  const card = await findCard();

  expect(within(card).getByText("Could not reach GitHub. Check the connection and try again.")).toBeVisible();
  await userEvent.click(within(card).getByRole("button", { name: "Download" }));
  expect(download).toHaveBeenCalledOnce();
});

test("a download shows how far it got", async () => {
  renderCard({ state: "downloading", currentVersion: "0.1.0", version: "0.2.0", percent: 42.4 });

  const card = await findCard();

  expect(within(card).getByText("Downloading 0.2.0")).toBeVisible();
  expect(within(card).getByText("42%")).toBeVisible();
  expect(within(card).getByRole("progressbar", { name: "Update downloaded" })).toHaveAttribute("aria-valuenow", "42");
});

test("a version that is ready asks for the restart", async () => {
  const install = vi.spyOn(bridge.updates, "install").mockResolvedValue(undefined);
  renderCard({ state: "downloaded", currentVersion: "0.1.0", version: "0.2.0" });

  const card = await findCard();

  expect(within(card).getByText("Babysitter 0.2.0 is ready")).toBeVisible();
  await userEvent.click(within(card).getByRole("button", { name: "Restart to update" }));
  expect(install).toHaveBeenCalledOnce();
});

test("a restart that failed says why and offers the restart again", async () => {
  renderCard({
    state: "downloaded",
    currentVersion: "0.1.0",
    version: "0.2.0",
    message: "ShipIt could not replace the app",
  });

  const card = await findCard();

  expect(within(card).getByText("ShipIt could not replace the app")).toBeVisible();
  expect(within(card).getByRole("button", { name: "Restart to update" })).toBeEnabled();
});

test("a restart in progress cannot start twice", async () => {
  renderCard({ state: "installing", currentVersion: "0.1.0", version: "0.2.0" });

  const card = await findCard();

  expect(within(card).getByRole("button", { name: /Restarting/ })).toBeDisabled();
});
