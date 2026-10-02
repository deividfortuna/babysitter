import type { ReactNode } from "react";
import { CheckIcon, CircleAlertIcon, CopyIcon, ExternalLinkIcon, FolderPlusIcon, InfoIcon } from "lucide-react";
import { GitHubIcon } from "@/components/github-icon";
import { SettingsCard, SettingsError, SettingsSection } from "@/components/settings-page";
import { LiveDot } from "@/components/status-badges";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import {
  useAuth,
  useCancelSignIn,
  useSignOut,
  useStartSignIn,
  type Auth,
  type AuthInstallation,
  type SignInPrompt,
  type TokenOrigin,
} from "@/hooks/useAuth";
import { useNow } from "@/hooks/use-now";
import { useCopy } from "@/hooks/useCopy";
import { avatarSource } from "@/lib/avatar";
import { initials } from "@/lib/initials";
import { cn } from "@/lib/utils";

type StartSignIn = ReturnType<typeof useStartSignIn>;
type SignOut = ReturnType<typeof useSignOut>;

const SIGN_IN_FAILURES: Record<string, string> = {
  expired: "The code expired before it was entered on GitHub.",
  denied: "The sign in was cancelled on GitHub.",
};

const TOKENS_FIRST: Partial<Record<TokenOrigin, { title: string; body: string }>> = {
  env: {
    title: "GITHUB_TOKEN comes first",
    body: "The daemon started with GITHUB_TOKEN in its environment, so it uses that token and not the app. Unset it and restart the daemon to use the app.",
  },
  flag: {
    title: "--token comes first",
    body: "The daemon started with the --token flag, so it uses that token and not the app. Start it without the flag to use the app.",
  },
};

const SIGNED_OUT_NOTES: Partial<Record<TokenOrigin, string>> = {
  gh: "Until you sign in, the daemon uses the token of the gh CLI, which reaches every repository you can.",
  "": "No token was found, so the daemon cannot reach GitHub until you sign in.",
};

function Title({ children }: { children: ReactNode }) {
  return <span className="text-title/5 font-medium">{children}</span>;
}

function Description({ children }: { children: ReactNode }) {
  return <p className="text-body/4.5 text-muted-foreground">{children}</p>;
}

type IconCardProps = {
  icon: ReactNode;
  title: string;
  description: ReactNode;
  action?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
};

function IconCard({ icon, title, description, action, children, footer }: IconCardProps) {
  return (
    <SettingsCard>
      <div className="flex items-start gap-4 p-5">
        <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-background [&_svg]:size-5">
          {icon}
        </div>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <Title>{title}</Title>
          <Description>{description}</Description>
          {children}
        </div>
        {action}
      </div>
      {footer}
    </SettingsCard>
  );
}

function Note({ icon: Icon = InfoIcon, children }: { icon?: typeof InfoIcon; children: ReactNode }) {
  return (
    <p className="flex items-start gap-2 px-0.5 text-body/4.5 text-muted-foreground">
      <Icon className="mt-px size-4 shrink-0" />
      <span>{children}</span>
    </p>
  );
}

function AccountAvatar({ login, src, className }: { login: string; src?: string; className?: string }) {
  return (
    <Avatar className={cn("border", className)}>
      {src ? <AvatarImage src={src} alt="" /> : null}
      <AvatarFallback className="bg-accent text-3xs font-semibold text-muted-foreground">
        {initials(login)}
      </AvatarFallback>
    </Avatar>
  );
}

function failureOf(auth: Auth, start: StartSignIn): string | undefined {
  if (start.error) return start.error.message;
  if (!auth.signInFailure) return undefined;
  return SIGN_IN_FAILURES[auth.signInFailure] ?? auth.signInError;
}

const steps = ["Enter a short code on github.com", "Choose the repositories", "babysitter watches their pull requests"];

function ConnectCard({ auth, start }: { auth: Auth; start: StartSignIn }) {
  const failure = failureOf(auth, start);
  const label = auth.signInFailure === "expired" ? "Get a new code" : "Sign in with GitHub";
  return (
    <IconCard
      icon={<GitHubIcon />}
      title="Connect your GitHub account"
      description="Sign in with the babysitter GitHub App. The daemon then reaches only the repositories you choose, and keeps no personal token."
      action={
        <Button disabled={start.isPending} onClick={() => start.mutate()}>
          {start.isPending ? <Spinner data-icon="inline-start" /> : null}
          {label}
        </Button>
      }
      footer={
        <ol className="grid grid-cols-3 gap-4 px-5 py-3.5">
          {steps.map((step, index) => (
            <li key={step} className="flex flex-col gap-1">
              <span className="eyebrow">Step {index + 1}</span>
              <span className="text-body/4.5">{step}</span>
            </li>
          ))}
        </ol>
      }
    >
      {failure ? <FailureLine message={failure} /> : null}
    </IconCard>
  );
}

function NotInBuild() {
  return (
    <IconCard
      icon={<GitHubIcon />}
      title="Not in this build"
      description="This build of babysitter has no GitHub App."
    />
  );
}

function useSecondsLeft(until: string) {
  const now = useNow(1_000);
  return Math.max(0, Math.round((Date.parse(until) - now) / 1_000));
}

function clock(seconds: number) {
  const minutes = Math.floor(seconds / 60);
  return `${minutes}:${String(seconds % 60).padStart(2, "0")}`;
}

function CodeCard({ prompt }: { prompt: SignInPrompt }) {
  const { copied, copy } = useCopy();
  const cancel = useCancelSignIn();
  const secondsLeft = useSecondsLeft(prompt.expiresAt);
  return (
    <SettingsCard className="divide-y-0">
      <div className="flex items-start gap-4 px-5 pt-5 pb-4">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <Title>Enter this code on GitHub</Title>
          <Description>GitHub asks for it to connect babysitter to your account.</Description>
        </div>
        <Button variant="ghost" size="sm" disabled={cancel.isPending} onClick={() => cancel.mutate()}>
          Cancel
        </Button>
      </div>
      <div className="flex items-center gap-3 px-5 pb-5">
        <div className="flex items-center gap-1 rounded-lg border bg-background py-1 pr-1 pl-4">
          <code aria-label="Sign in code" className="font-mono text-2xl/9 font-medium tracking-widest">
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
        </div>
        <Button asChild>
          <a href={prompt.verificationUri} target="_blank" rel="noreferrer" onClick={() => void copy(prompt.userCode)}>
            Copy code and open GitHub
            <ExternalLinkIcon data-icon="inline-end" />
          </a>
        </Button>
      </div>
      <div role="status" className="flex items-center gap-2.5 rounded-b-lg border-t bg-muted/60 px-5 py-2.5">
        <LiveDot tone="attention" title="" className="size-2" />
        <span className="flex-1 text-body/4.5 text-muted-foreground">
          Waiting for you to approve on GitHub. This page moves on by itself.
        </span>
        <span className="font-mono text-2xs text-muted-foreground">expires in {clock(secondsLeft)}</span>
      </div>
    </SettingsCard>
  );
}

type AccountCardProps = {
  auth: Auth;
  badge: ReactNode;
  description: string;
  alert?: string;
  children: ReactNode;
};

function FailureLine({ message }: { message: string }) {
  return (
    <p role="alert" className="mt-1.5 flex items-start gap-1.5 text-body/4.5 text-destructive">
      <CircleAlertIcon className="mt-px size-4 shrink-0" />
      <span>{message}</span>
    </p>
  );
}

function AccountCard({ auth, badge, description, alert, children }: AccountCardProps) {
  const login = auth.login ?? "";
  return (
    <SettingsCard>
      <div className="flex items-center gap-3.5 px-5 py-4">
        <AccountAvatar login={login} src={auth.avatarUrl} className="size-10" />
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <div className="flex items-center gap-2">
            <span className="text-title/5 font-medium">{login}</span>
            {badge}
          </div>
          <span className="text-body/4.5 text-muted-foreground">{description}</span>
          {alert ? <FailureLine message={alert} /> : null}
        </div>
        <div className="flex gap-2">{children}</div>
      </div>
    </SettingsCard>
  );
}

function SignOutButton({ signOut, variant = "outline" }: { signOut: SignOut; variant?: "outline" | "ghost" }) {
  return (
    <Button variant={variant} size="sm" disabled={signOut.isPending} onClick={() => signOut.mutate()}>
      {signOut.isPending ? <Spinner data-icon="inline-start" /> : null}
      Sign out
    </Button>
  );
}

function StateBadge({ label, pip }: { label: string; pip: string }) {
  return (
    <Badge variant="outline" className="h-5 gap-1.5 text-foreground">
      <span aria-hidden className={cn("size-1.5 rounded-full", pip)} />
      {label}
    </Badge>
  );
}

function ChooseRepositories({ installUrl }: { installUrl: string }) {
  return (
    <IconCard
      icon={<FolderPlusIcon />}
      title="Choose the repositories babysitter may watch"
      description="The app is not installed on any account yet, so the daemon cannot reach a repository. Install it on GitHub, then come back here."
      action={
        <Button asChild>
          <a href={installUrl} target="_blank" rel="noreferrer">
            Choose repositories
            <ExternalLinkIcon data-icon="inline-end" />
          </a>
        </Button>
      }
    />
  );
}

function InstallationRow({ installation }: { installation: AuthInstallation }) {
  return (
    <div className="flex items-center gap-2.5 px-5 py-3">
      <AccountAvatar
        login={installation.login}
        src={avatarSource(installation.login, false, installation.avatarUrl)}
        className="size-5"
      />
      <span className="flex-1 text-sm font-medium">{installation.login}</span>
      <span className="font-mono text-2xs text-muted-foreground">
        {installation.organization ? "organization" : "personal account"}
      </span>
    </div>
  );
}

function InstallationsUnknown({ reason }: { reason: string }) {
  return (
    <div role="alert" className="flex items-start gap-2.5 px-5 py-3">
      <CircleAlertIcon className="mt-0.5 size-4 shrink-0 text-destructive" />
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-sm font-medium">Could not read the accounts the app is installed on</span>
        <span className="text-body/4.5 wrap-break-word text-muted-foreground">{reason}</span>
      </div>
    </div>
  );
}

function Installations({ auth }: { auth: Auth }) {
  const installations = auth.installations ?? [];
  if (auth.installationsError) {
    return (
      <InstalledOn installUrl={auth.installUrl}>
        <InstallationsUnknown reason={auth.installationsError} />
      </InstalledOn>
    );
  }
  if (installations.length === 0) return <ChooseRepositories installUrl={auth.installUrl} />;
  return (
    <InstalledOn installUrl={auth.installUrl}>
      {installations.map((installation) => (
        <InstallationRow key={installation.login} installation={installation} />
      ))}
    </InstalledOn>
  );
}

function InstalledOn({ installUrl, children }: { installUrl: string; children: ReactNode }) {
  return (
    <SettingsSection label="Installed on">
      <SettingsCard>
        {children}
        <div className="flex items-center gap-3 px-5 py-3">
          <p className="flex-1 text-body/4.5 text-muted-foreground">
            The daemon reaches only the repositories you installed the app on in these accounts.
          </p>
          <Button asChild variant="outline" size="sm">
            <a href={installUrl} target="_blank" rel="noreferrer">
              Manage on GitHub
              <ExternalLinkIcon data-icon="inline-end" />
            </a>
          </Button>
        </div>
      </SettingsCard>
    </SettingsSection>
  );
}

function Connected({ auth, signOut }: { auth: Auth; signOut: SignOut }) {
  return (
    <>
      <AccountCard
        auth={auth}
        badge={<StateBadge label="connected" pip="bg-success" />}
        description="Signed in with the babysitter GitHub App"
      >
        <SignOutButton signOut={signOut} />
      </AccountCard>
      <Installations auth={auth} />
    </>
  );
}

function NotInUse({ auth, signOut }: { auth: Auth; signOut: SignOut }) {
  return (
    <AccountCard
      auth={auth}
      badge={<StateBadge label="not in use" pip="bg-muted-foreground" />}
      description="Signed in with the babysitter GitHub App"
    >
      <SignOutButton signOut={signOut} />
    </AccountCard>
  );
}

function Unreachable({ auth, signOut }: { auth: Auth; signOut: SignOut }) {
  const recheck = useAuth();
  return (
    <AccountCard
      auth={auth}
      badge={<StateBadge label="cannot reach GitHub" pip="bg-destructive" />}
      description="Signed in, but the daemon cannot get the token of the app right now."
    >
      <SignOutButton signOut={signOut} variant="ghost" />
      <Button variant="outline" size="sm" disabled={recheck.isFetching} onClick={() => void recheck.refetch()}>
        {recheck.isFetching ? <Spinner data-icon="inline-start" /> : null}
        Try again
      </Button>
    </AccountCard>
  );
}

function Expired({ auth, signOut, start }: { auth: Auth; signOut: SignOut; start: StartSignIn }) {
  return (
    <>
      <AccountCard
        auth={auth}
        badge={
          <Badge variant="destructive" className="h-5">
            sign in expired
          </Badge>
        }
        description="GitHub did not renew the token of the app. Sign in again to keep watching pull requests."
        alert={failureOf(auth, start)}
      >
        <SignOutButton signOut={signOut} variant="ghost" />
        <Button size="sm" disabled={start.isPending} onClick={() => start.mutate()}>
          {start.isPending ? <Spinner data-icon="inline-start" /> : null}
          Sign in again
        </Button>
      </AccountCard>
      <Note icon={CircleAlertIcon}>
        The daemon does not fall back to another token, so it cannot reach GitHub until you sign in again.
      </Note>
    </>
  );
}

function SignedOut({ auth, start }: { auth: Auth; start: StartSignIn }) {
  const note = SIGNED_OUT_NOTES[auth.origin];
  return (
    <>
      {auth.appAvailable ? <ConnectCard auth={auth} start={start} /> : <NotInBuild />}
      {note ? <Note>{note}</Note> : null}
    </>
  );
}

function TokenFirst({ origin }: { origin: TokenOrigin }) {
  const token = TOKENS_FIRST[origin];
  if (!token) return null;
  return (
    <div className="flex items-start gap-3 rounded-lg border bg-background px-4 py-3">
      <InfoIcon className="mt-0.5 size-4 shrink-0" />
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium">{token.title}</span>
        <Description>{token.body}</Description>
      </div>
    </div>
  );
}

function AccessState({ auth }: { auth: Auth }) {
  const start = useStartSignIn();
  const signOut = useSignOut();
  const views: Record<Auth["state"], ReactNode> = {
    signed_out: <SignedOut auth={auth} start={start} />,
    waiting: auth.signIn ? <CodeCard prompt={auth.signIn} /> : null,
    connected: <Connected auth={auth} signOut={signOut} />,
    not_in_use: <NotInUse auth={auth} signOut={signOut} />,
    expired: <Expired auth={auth} signOut={signOut} start={start} />,
    unreachable: <Unreachable auth={auth} signOut={signOut} />,
  };
  return (
    <>
      {views[auth.state]}
      <SettingsError message={signOut.error?.message} />
    </>
  );
}

function Loading() {
  return (
    <div className="flex flex-col gap-4.5">
      <span className="sr-only">Loading the GitHub access of the daemon…</span>
      <SettingsCard>
        <div className="flex items-center gap-3.5 px-5 py-4">
          <Skeleton className="size-10 rounded-full" />
          <div className="flex flex-1 flex-col gap-2">
            <Skeleton className="h-3.5 w-36" />
            <Skeleton className="h-3 w-64" />
          </div>
          <Skeleton className="h-8 w-20" />
        </div>
      </SettingsCard>
      <SettingsSection label="Installed on">
        <SettingsCard>
          <div className="flex items-center gap-2.5 px-5 py-3">
            <Skeleton className="size-5 rounded-full" />
            <Skeleton className="h-3 w-30" />
          </div>
        </SettingsCard>
      </SettingsSection>
    </div>
  );
}

export function GitHubPanel() {
  const auth = useAuth();
  if (auth.error) return <SettingsError message={auth.error.message} />;
  if (!auth.data) return <Loading />;
  return (
    <div className="flex flex-col gap-4.5">
      <TokenFirst origin={auth.data.origin} />
      <AccessState auth={auth.data} />
      <SettingsError message={auth.data.error} />
    </div>
  );
}
