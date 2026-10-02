import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { renderWithProviders } from "@test/test-utils";
import { bridge } from "@/lib/bridge";
import { versionWarning } from "../../shared/connections";
import { VersionWarning } from "./version-warning";

afterEach(() => {
  vi.restoreAllMocks();
});

test("a daemon on another version shows a warning that tells both versions", async () => {
  vi.spyOn(bridge.app, "getVersion").mockResolvedValue("0.2.0");

  renderWithProviders(<VersionWarning name="studio" version="0.1.0" />);

  const icon = await screen.findByRole("img", { name: versionWarning("studio", "0.1.0", "0.2.0") });
  await userEvent.hover(icon);
  expect(await screen.findByRole("tooltip")).toHaveTextContent("studio runs babysitter 0.1.0, and this app is 0.2.0.");
});

test("a daemon on the version of the app, or of no known version, shows no warning", async () => {
  const getVersion = vi.spyOn(bridge.app, "getVersion").mockResolvedValue("0.2.0");

  renderWithProviders(
    <>
      <VersionWarning name="same" version="v0.2.0" />
      <VersionWarning name="unknown" />
    </>,
  );

  await vi.waitFor(() => expect(getVersion).toHaveBeenCalled());
  expect(screen.queryByRole("img")).toBeNull();
});
