import { CheckIcon, CopyIcon, ExternalLinkIcon } from "lucide-react";
import { SettingsCard, SettingsError, SettingsRow, SettingsSection } from "@/components/settings-page";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  useAuth,
  useCancelSignIn,
  useSignOut,
  useStartSignIn,
  type Auth,
  type SignInPrompt,
  type TokenOrigin,
} from "@/hooks/useAuth";
import { useCopy } from "@/hooks/useCopy";

const ORIGINS: Record<TokenOrigin, { label: string; description: string }> = {
  app: {
    label: "babysitter GitHub App",
    description: "The daemon acts as you, only on the repositories where the app is installed.",
  },
  gh: {
    label: "gh CLI",
    description: "The token of 'gh auth login'. The daemon acts as you on every repository you can reach.",
  },
  env: { label: "GITHUB_TOKEN", description: "The token in the environment of the daemon. It comes before the app." },
  flag: { label: "--token flag", description: "The token the daemon was started with. It comes before the app." },
  "": { label: "No token", description: "Sign in with the GitHub App below, or run 'gh auth login'." },
};

function TokenSource({ auth }: { auth: Auth }) {
  const origin = ORIGINS[auth.origin];
  return (
    <SettingsSection label="Token">
      <SettingsCard>
        <SettingsRow label={origin.label} description={origin.description} />
      </SettingsCard>
    </SettingsSection>
  );
}

function CodePrompt({ prompt }: { prompt: SignInPrompt }) {
  const { copied, copy } = useCopy();
  const cancel = useCancelSignIn();
  return (
    <SettingsRow
      label="Enter this code on GitHub"
      description={
        <span className="inline-flex items-center gap-1.5">
          <Spinner className="size-3" />
          Waiting for GitHub. The daemon signs in when you accept.
        </span>
      }
    >
      <div className="flex items-center gap-2">
        <code
          aria-label="Sign in code"
          className="rounded-md border bg-background px-2 py-1 font-mono text-sm tracking-widest"
        >
          {prompt.userCode}
        </code>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={copied ? "Copied" : "Copy the code"}
          onClick={() => void copy(prompt.userCode)}
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
        </Button>
        <Button asChild size="sm">
          <a href={prompt.verificationUri} target="_blank" rel="noreferrer">
            Open GitHub
            <ExternalLinkIcon data-icon="inline-end" />
          </a>
        </Button>
        <Button variant="outline" size="sm" disabled={cancel.isPending} onClick={() => cancel.mutate()}>
          Cancel
        </Button>
      </div>
    </SettingsRow>
  );
}

function installedOn(installations: string[] | null) {
  if (!installations?.length) return "Not installed on any account yet.";
  return `Installed on ${installations.join(", ")}.`;
}

function usageOf(auth: Auth) {
  if (auth.origin === "app") return installedOn(auth.installations);
  if (auth.origin === "") return "The sign in no longer works. Sign out, then sign in again.";
  return "Not in use: the token above comes first.";
}

type SignedInProps = { auth: Auth; signOut: ReturnType<typeof useSignOut> };

function SignedIn({ auth, signOut }: SignedInProps) {
  const usage = usageOf(auth);
  return (
    <>
      <SettingsRow label={`Signed in as @${auth.login ?? ""}`} description={usage}>
        <Button variant="outline" size="sm" disabled={signOut.isPending} onClick={() => signOut.mutate()}>
          {signOut.isPending ? <Spinner data-icon="inline-start" /> : null}
          Sign out
        </Button>
      </SettingsRow>
      <SettingsRow
        label="Repositories"
        description="The app reaches only the repositories you install it on. Add or remove them on GitHub."
      >
        <Button asChild variant="outline" size="sm">
          <a href={auth.installUrl} target="_blank" rel="noreferrer">
            Choose repositories
            <ExternalLinkIcon data-icon="inline-end" />
          </a>
        </Button>
      </SettingsRow>
    </>
  );
}

function SignedOut({ start }: { start: ReturnType<typeof useStartSignIn> }) {
  return (
    <SettingsRow
      label="Sign in with the GitHub App"
      description="Give babysitter access only to the repositories you choose, with no personal token. The gh CLI and GITHUB_TOKEN still work."
    >
      <Button size="sm" disabled={start.isPending} onClick={() => start.mutate()}>
        {start.isPending ? <Spinner data-icon="inline-start" /> : null}
        Sign in
      </Button>
    </SettingsRow>
  );
}

function AppRows({ auth }: { auth: Auth }) {
  const start = useStartSignIn();
  const signOut = useSignOut();
  const rows = auth.signedIn ? <SignedIn auth={auth} signOut={signOut} /> : <SignedOut start={start} />;
  return (
    <>
      <SettingsCard>{rows}</SettingsCard>
      <SettingsError message={start.error?.message ?? signOut.error?.message} />
    </>
  );
}

function AppAccess({ auth }: { auth: Auth }) {
  const outOfBuild = !auth.appAvailable && !auth.signedIn;
  if (outOfBuild) {
    return (
      <SettingsCard>
        <SettingsRow label="Not in this build" description="This build of babysitter has no GitHub App." />
      </SettingsCard>
    );
  }
  if (auth.signIn) {
    return (
      <SettingsCard>
        <CodePrompt prompt={auth.signIn} />
      </SettingsCard>
    );
  }
  return <AppRows auth={auth} />;
}

export function GitHubPanel() {
  const auth = useAuth();
  if (auth.error) return <SettingsError message={auth.error.message} />;
  if (!auth.data) return <Spinner />;
  return (
    <div className="flex flex-col gap-4.5">
      <TokenSource auth={auth.data} />
      <SettingsSection label="GitHub App">
        <AppAccess auth={auth.data} />
      </SettingsSection>
      <SettingsError message={auth.data.error} />
    </div>
  );
}
