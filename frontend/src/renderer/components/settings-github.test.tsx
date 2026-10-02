import { act, screen, waitFor, within } from "@testing-library/react";
import { focusManager } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi } from "@test/msw";
import { buildAuth } from "@test/fixtures";
import { renderWithProviders } from "@test/test-utils";
import type { Auth } from "@/hooks/useAuth";
import { GitHubPanel } from "./settings-github";
import { SettingsDialog } from "./settings-dialog";

const installUrl = "https://github.com/apps/babysitter-orchestrator/installations/new";

const prompt = {
  userCode: "WDJB-MJHT",
  verificationUri: "https://github.com/login/device",
  expiresAt: "2026-10-02T09:15:00Z",
};

const connected = buildAuth({
  state: "connected",
  origin: "app",
  login: "deividfortuna",
  avatarUrl: "https://avatars.githubusercontent.com/u/1",
  installations: [
    { login: "deividfortuna", avatarUrl: "https://avatars.githubusercontent.com/u/1", organization: false },
    { login: "acme", avatarUrl: "https://avatars.githubusercontent.com/u/2", organization: true },
  ],
});

function serveAuthSequence(states: Auth[]) {
  let current = states[0];
  server.use(http.get(apiUrl("/api/v1/auth"), () => HttpResponse.json(current)));
  return (next: number) => {
    current = states[next];
  };
}

afterEach(() => {
  vi.useRealTimers();
});

test("the GitHub page sits in the App group", async () => {
  serveApi();
  renderWithProviders(<SettingsDialog open category="github" onOpenChange={vi.fn()} />);

  expect(await screen.findByRole("button", { name: "Sign in with GitHub" })).toBeVisible();
  expect(screen.getByRole("heading", { name: "GitHub" })).toBeVisible();
  expect(screen.getByText("How babysitter connects to your GitHub account.")).toBeVisible();
  expect(screen.getByRole("group", { name: "App" })).toHaveTextContent("GitHub");
});

test("signed out offers the app, its steps and the token in use until then", async () => {
  serveApi({ auth: buildAuth({ origin: "gh" }) });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("Connect your GitHub account")).toBeVisible();
  const steps = within(screen.getByRole("list"));
  expect(steps.getAllByRole("listitem").map((step) => step.textContent)).toEqual([
    "Step 1Enter a short code on github.com",
    "Step 2Choose the repositories",
    "Step 3babysitter watches their pull requests",
  ]);
  expect(
    screen.getByText(
      "Until you sign in, the daemon uses the token of the gh CLI, which reaches every repository you can.",
    ),
  ).toBeVisible();
});

test("signed out without any token says the daemon cannot reach GitHub", async () => {
  serveApi({ auth: buildAuth({ origin: "" }) });
  renderWithProviders(<GitHubPanel />);

  expect(
    await screen.findByText("No token was found, so the daemon cannot reach GitHub until you sign in."),
  ).toBeVisible();
});

test("sign in shows the code, the link to GitHub and the time left", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-02T09:00:08Z"));
  serveApi();
  const setAuth = serveAuthSequence([buildAuth(), buildAuth({ state: "waiting", signIn: prompt })]);
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

  await user.click(await screen.findByRole("button", { name: "Sign in with GitHub" }));

  expect(await screen.findByLabelText("Sign in code")).toHaveTextContent("WDJB-MJHT");
  expect(screen.getByRole("link", { name: "Copy code and open GitHub" })).toHaveAttribute(
    "href",
    "https://github.com/login/device",
  );
  expect(screen.getByRole("status")).toHaveTextContent("Waiting for you to approve on GitHub.");
  expect(screen.getByRole("status")).toHaveTextContent("expires in 14:52");
  expect(started).toBe(1);
});

test("cancel stops the sign in", async () => {
  serveApi();
  const setAuth = serveAuthSequence([buildAuth({ state: "waiting", signIn: prompt }), buildAuth()]);
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

  expect(await screen.findByRole("button", { name: "Sign in with GitHub" })).toBeVisible();
  expect(cancelled).toBe(1);
});

test("a cancel that fails shows what the daemon answered", async () => {
  serveApi();
  serveAuthSequence([buildAuth({ state: "waiting", signIn: prompt })]);
  server.use(
    http.delete(apiUrl("/api/v1/auth/signin"), () =>
      HttpResponse.json(
        { error: { code: "internal", message: "the daemon could not stop the sign in" } },
        { status: 500 },
      ),
    ),
  );
  const user = userEvent.setup();
  renderWithProviders(<GitHubPanel />);

  await user.click(await screen.findByRole("button", { name: "Cancel" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("the daemon could not stop the sign in");
});

test("an expired code asks for a new one", async () => {
  serveApi({ auth: buildAuth({ signInFailure: "expired", signInError: "the code expired" }) });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByRole("alert")).toHaveTextContent("The code expired before it was entered on GitHub.");
  expect(screen.getByRole("button", { name: "Get a new code" })).toBeVisible();
});

test("a refused code says so and offers the sign in again", async () => {
  serveApi({ auth: buildAuth({ signInFailure: "denied", signInError: "the sign in was cancelled on GitHub" }) });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByRole("alert")).toHaveTextContent("The sign in was cancelled on GitHub.");
  expect(screen.getByRole("button", { name: "Sign in with GitHub" })).toBeVisible();
});

test("connected names the account and the accounts the app is installed on", async () => {
  serveApi({ auth: connected });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("connected")).toBeVisible();
  expect(screen.getByText("Signed in with the babysitter GitHub App")).toBeVisible();
  expect(screen.getByRole("heading", { name: "Installed on" })).toBeVisible();
  expect(screen.getByText("personal account")).toBeVisible();
  expect(screen.getByText("organization")).toBeVisible();
  expect(screen.getByText("acme")).toBeVisible();
  expect(screen.getByRole("link", { name: "Manage on GitHub" })).toHaveAttribute("href", installUrl);
});

test("a sign in whose token cannot be read is never shown as connected", async () => {
  serveApi({
    auth: {
      ...connected,
      state: "unreachable",
      origin: "",
      installations: [],
      error: "renew the app token: GitHub answered 502",
    },
  });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("cannot reach GitHub")).toBeVisible();
  expect(screen.queryByText("connected")).not.toBeInTheDocument();
  expect(screen.getByText("renew the app token: GitHub answered 502")).toBeVisible();
  expect(screen.getByRole("button", { name: "Try again" })).toBeVisible();
});

test("coming back to the app reads the installations again", async () => {
  serveApi();
  const setAuth = serveAuthSequence([{ ...connected, installations: [] }, connected]);
  renderWithProviders(<GitHubPanel />);
  expect(await screen.findByText("Choose the repositories babysitter may watch")).toBeVisible();

  setAuth(1);
  act(() => {
    focusManager.setFocused(false);
    focusManager.setFocused(true);
  });

  expect(await screen.findByRole("heading", { name: "Installed on" })).toBeVisible();
  focusManager.setFocused(undefined);
});

test("installations that cannot be read are not called missing", async () => {
  serveApi({
    auth: { ...connected, installations: [], installationsError: "list the installations of the GitHub App: 502" },
  });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByRole("alert")).toHaveTextContent("Could not read the accounts the app is installed on");
  expect(screen.getByText("list the installations of the GitHub App: 502")).toBeVisible();
  expect(screen.getByRole("link", { name: "Manage on GitHub" })).toHaveAttribute("href", installUrl);
  expect(screen.queryByText("Choose the repositories babysitter may watch")).not.toBeInTheDocument();
});

test("an expired sign in shows why signing in again failed", async () => {
  serveApi({
    auth: { ...connected, state: "expired", origin: "", installations: [], signInFailure: "denied", signInError: "x" },
  });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByRole("alert")).toHaveTextContent("The sign in was cancelled on GitHub.");
  expect(screen.getByRole("button", { name: "Sign in again" })).toBeVisible();
});

test("connected without an installation asks to choose the repositories", async () => {
  serveApi({ auth: { ...connected, installations: [] } });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("Choose the repositories babysitter may watch")).toBeVisible();
  expect(screen.getByRole("link", { name: "Choose repositories" })).toHaveAttribute("href", installUrl);
  expect(screen.queryByRole("heading", { name: "Installed on" })).not.toBeInTheDocument();
});

test("a token in the environment comes before the app", async () => {
  serveApi({ auth: { ...connected, state: "not_in_use", origin: "env", installations: [] } });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("GITHUB_TOKEN comes first")).toBeVisible();
  expect(screen.getByText("not in use")).toBeVisible();
  expect(screen.getByRole("button", { name: "Sign out" })).toBeVisible();
});

test("the --token flag comes before the app", async () => {
  serveApi({ auth: { ...connected, state: "not_in_use", origin: "flag", installations: [] } });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("--token comes first")).toBeVisible();
});

test("an expired sign in offers to sign in again", async () => {
  serveApi();
  const setAuth = serveAuthSequence([
    { ...connected, state: "expired", origin: "", installations: [] },
    { ...connected, state: "waiting", origin: "", installations: [], signIn: prompt },
  ]);
  server.use(
    http.post(apiUrl("/api/v1/auth/signin"), () => {
      setAuth(1);
      return HttpResponse.json(prompt, { status: 202 });
    }),
  );
  const user = userEvent.setup();
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("sign in expired")).toBeVisible();
  expect(
    screen.getByText("GitHub did not renew the token of the app. Sign in again to keep watching pull requests."),
  ).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Sign in again" }));

  expect(await screen.findByLabelText("Sign in code")).toHaveTextContent("WDJB-MJHT");
});

test("sign out goes back to the next source", async () => {
  serveApi();
  const setAuth = serveAuthSequence([connected, buildAuth({ origin: "gh" })]);
  server.use(
    http.post(apiUrl("/api/v1/auth/signout"), () => {
      setAuth(1);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  const user = userEvent.setup();
  renderWithProviders(<GitHubPanel />);

  await user.click(await screen.findByRole("button", { name: "Sign out" }));

  expect(await screen.findByRole("button", { name: "Sign in with GitHub" })).toBeVisible();
});

test("a refused sign in shows what the daemon answered", async () => {
  serveApi();
  server.use(
    http.post(apiUrl("/api/v1/auth/signin"), () =>
      HttpResponse.json(
        { error: { code: "signin_failed", message: "ask GitHub for a device code: GitHub answered 502" } },
        { status: 500 },
      ),
    ),
  );
  const user = userEvent.setup();
  renderWithProviders(<GitHubPanel />);

  await user.click(await screen.findByRole("button", { name: "Sign in with GitHub" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("GitHub answered 502");
});

test("a build without the app says so", async () => {
  serveApi({ auth: buildAuth({ appAvailable: false }) });
  renderWithProviders(<GitHubPanel />);

  expect(await screen.findByText("This build of babysitter has no GitHub App.")).toBeVisible();
  await waitFor(() => expect(screen.queryByRole("button", { name: "Sign in with GitHub" })).not.toBeInTheDocument());
});

test("loading shows the shape of the page", () => {
  serveApi();
  renderWithProviders(<GitHubPanel />);

  expect(screen.getByText("Loading the GitHub access of the daemon…")).toBeInTheDocument();
});
