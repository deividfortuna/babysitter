import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi } from "@test/msw";
import { buildAuth } from "@test/fixtures";
import { renderWithProviders } from "@test/test-utils";
import type { Auth } from "@/hooks/useAuth";
import { GitHubPanel } from "./settings-github";
import { SettingsDialog } from "./settings-dialog";

const prompt = {
  userCode: "WDJB-MJHT",
  verificationUri: "https://github.com/login/device",
  expiresAt: "2026-10-02T09:15:00Z",
};

function serveAuthSequence(states: Auth[]) {
  let current = states[0];
  server.use(http.get(apiUrl("/api/v1/auth"), () => HttpResponse.json(current)));
  return (next: number) => {
    current = states[next];
  };
}

test("the GitHub access page lives under the daemon", async () => {
  serveApi();
  renderWithProviders(<SettingsDialog open category="github" onOpenChange={vi.fn()} />);

  expect(await screen.findByRole("button", { name: "Sign in" })).toBeVisible();
  expect(screen.getByRole("group", { name: "Daemon" })).toHaveTextContent("GitHub access");
});

test("names the gh CLI as the source and offers the app next to it", async () => {
  serveApi({ auth: buildAuth({ origin: "gh" }) });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("gh CLI")).toBeVisible();
  expect(screen.getByText(/The gh CLI and GITHUB_TOKEN still work/)).toBeVisible();
});

test("sign in shows the code and the link to GitHub", async () => {
  serveApi();
  const setAuth = serveAuthSequence([buildAuth(), buildAuth({ signIn: prompt })]);
  let started = 0;
  server.use(
    http.post(apiUrl("/api/v1/auth/signin"), () => {
      started += 1;
      setAuth(1);
      return HttpResponse.json(prompt, { status: 202 });
    }),
  );
  const user = userEvent.setup();
  renderWithProviders(<GitHubPanel />);

  await user.click(await screen.findByRole("button", { name: "Sign in" }));

  expect(await screen.findByLabelText("Sign in code")).toHaveTextContent("WDJB-MJHT");
  expect(screen.getByRole("link", { name: "Open GitHub" })).toHaveAttribute("href", "https://github.com/login/device");
  expect(started).toBe(1);
});

test("cancel stops the sign in", async () => {
  serveApi();
  const setAuth = serveAuthSequence([buildAuth({ signIn: prompt }), buildAuth()]);
  let cancelled = 0;
  server.use(
    http.delete(apiUrl("/api/v1/auth/signin"), () => {
      cancelled += 1;
      setAuth(1);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  const user = userEvent.setup();
  renderWithProviders(<GitHubPanel />);

  await user.click(await screen.findByRole("button", { name: "Cancel" }));

  expect(await screen.findByRole("button", { name: "Sign in" })).toBeVisible();
  expect(cancelled).toBe(1);
});

test("a signed in app names the account and the installations", async () => {
  serveApi({
    auth: buildAuth({ origin: "app", signedIn: true, login: "octocat", installations: ["octocat", "acme"] }),
  });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("Signed in as @octocat")).toBeVisible();
  expect(screen.getByText("babysitter GitHub App")).toBeVisible();
  expect(screen.getByText("Installed on octocat, acme.")).toBeVisible();
  expect(screen.getByRole("link", { name: "Choose repositories" })).toHaveAttribute(
    "href",
    "https://github.com/apps/babysitter/installations/new",
  );
});

test("a token in the environment comes before the app", async () => {
  serveApi({ auth: buildAuth({ origin: "env", signedIn: true, login: "octocat" }) });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("GITHUB_TOKEN")).toBeVisible();
  expect(screen.getByText("Not in use: the token above comes first.")).toBeVisible();
});

test("sign out goes back to the next source", async () => {
  serveApi();
  const setAuth = serveAuthSequence([
    buildAuth({ origin: "app", signedIn: true, login: "octocat", installations: ["octocat"] }),
    buildAuth({ origin: "gh" }),
  ]);
  server.use(
    http.post(apiUrl("/api/v1/auth/signout"), () => {
      setAuth(1);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  const user = userEvent.setup();
  renderWithProviders(<GitHubPanel />);

  await user.click(await screen.findByRole("button", { name: "Sign out" }));

  expect(await screen.findByText("gh CLI")).toBeVisible();
  expect(screen.getByRole("button", { name: "Sign in" })).toBeVisible();
});

test("a refused sign in shows what the daemon answered", async () => {
  serveApi({ auth: buildAuth({ appAvailable: true }) });
  server.use(
    http.post(apiUrl("/api/v1/auth/signin"), () =>
      HttpResponse.json(
        { error: { code: "app_unavailable", message: "this build of babysitter has no GitHub App" } },
        { status: 503 },
      ),
    ),
  );
  const user = userEvent.setup();
  renderWithProviders(<GitHubPanel />);

  await user.click(await screen.findByRole("button", { name: "Sign in" }));

  expect(await screen.findByText("this build of babysitter has no GitHub App")).toBeVisible();
});

test("an expired sign in shows the error of the daemon", async () => {
  serveApi({
    auth: buildAuth({
      origin: "",
      signedIn: true,
      login: "octocat",
      error: "the GitHub App sign in expired: run 'babysitter auth login' or sign in again in the app",
    }),
  });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText(/the GitHub App sign in expired/)).toBeVisible();
  expect(screen.getByText("No token")).toBeVisible();
  expect(screen.getByText("The sign in no longer works. Sign out, then sign in again.")).toBeVisible();
});

test("a build without the app says so", async () => {
  serveApi({ auth: buildAuth({ appAvailable: false }) });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("This build of babysitter has no GitHub App.")).toBeVisible();
  await waitFor(() => expect(screen.queryByRole("button", { name: "Sign in" })).not.toBeInTheDocument());
});
