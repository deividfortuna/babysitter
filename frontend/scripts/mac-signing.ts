type Env = Record<string, string | undefined>;

export type MacSigning = {
  osxSign?: {
    identity?: string;
    identityValidation?: boolean;
    continueOnError?: boolean;
    optionsForFile?: (filePath: string) => { hardenedRuntime: boolean };
  };
  osxNotarize?: { appleApiKey: string; appleApiKeyId: string; appleApiIssuer: string };
};

const AD_HOC_SIGN = {
  identity: "-",
  identityValidation: false,
  continueOnError: false,
  optionsForFile: () => ({ hardenedRuntime: false }),
};

function setting(env: Env, name: string): string | undefined {
  const value = env[name]?.trim();
  return value ? value : undefined;
}

function notaryKey(env: Env): MacSigning["osxNotarize"] {
  const appleApiKey = setting(env, "APPLE_API_KEY");
  const appleApiKeyId = setting(env, "APPLE_API_KEY_ID");
  const appleApiIssuer = setting(env, "APPLE_API_ISSUER");
  if (!appleApiKey || !appleApiKeyId || !appleApiIssuer) return undefined;
  return { appleApiKey, appleApiKeyId, appleApiIssuer };
}

function skipsSignature(env: Env): boolean {
  return setting(env, "BABYSITTER_SKIP_SIGN") === "1";
}

function signsWithIdentity(env: Env): boolean {
  return !skipsSignature(env) && setting(env, "BABYSITTER_SIGN_IDENTITY") !== undefined;
}

function isPreview(env: Env): boolean {
  return setting(env, "BABYSITTER_RELEASE_CHANNEL") === "preview";
}

export function updateResources(env: Env): string[] {
  return signsWithIdentity(env) && !isPreview(env) ? ["assets/app-update.yml"] : [];
}

export function macSigning(env: Env): MacSigning {
  if (skipsSignature(env)) return {};

  const identity = setting(env, "BABYSITTER_SIGN_IDENTITY");
  if (identity) {
    const osxSign = { identity, continueOnError: false };
    const osxNotarize = notaryKey(env);
    return osxNotarize ? { osxSign, osxNotarize } : { osxSign };
  }
  if (setting(env, "BABYSITTER_ADHOC_SIGN") === "1") return { osxSign: AD_HOC_SIGN };
  return { osxSign: {} };
}
