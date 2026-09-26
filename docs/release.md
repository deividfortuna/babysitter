# Release

A push of a tag that starts with `v` runs `.github/workflows/release.yaml`.
The workflow has four jobs. `cli` and `desktop` run in that order. `feed`
and `homebrew` start when `desktop` ends with success.

| Job | Runner | What it does |
| --- | --- | --- |
| `cli` | `ubuntu-latest` | GoReleaser makes the GitHub release and uploads the CLI archives and `checksums.txt` |
| `desktop` | `macos-latest` (arm64), `macos-15-intel` (x64) | Makes the app, checks it, and uploads the DMG and the zip to the release |
| `feed` | `ubuntu-latest` | Writes `latest-mac.yml` from the two zips and uploads it to the release. Only for a signed app |
| `homebrew` | `ubuntu-latest` | Writes `Casks/babysitter.rb` in the tap and pushes it |

## Versions

The tag is the version. `desktop` writes it into `frontend/package.json`
before it builds, so the app, its bundle version and
`babysitter --version` of the daemon inside the app all show the tag.
`main` keeps the version it has. You do not have to change
`package.json` before a release.

A tag that has a `-`, such as `v0.2.0-beta.1`, is a prerelease.
GoReleaser marks the GitHub release as a prerelease, and `homebrew`
does not run. Use only `alpha` or `beta` as the name of a prerelease.
The updater of the app reads any other name, such as `rc`, as a
channel of its own, and `feed` refuses it.

## Make a release

```sh
git tag v0.2.0
git push origin v0.2.0
```

To test the workflow with no release, run it from the Actions tab
(`workflow_dispatch`). `cli` makes a GoReleaser snapshot. `desktop`
makes the app at version `0.0.0-snapshot.<run>`. Both upload their
files as workflow artifacts. `homebrew` does not run.

## Assets of a release

For each arch, `arm64` and `x64`:

| Asset | Use |
| --- | --- |
| `Babysitter-<version>-darwin-<arch>.dmg` | The DMG of this version |
| `Babysitter-<version>-darwin-<arch>.zip` | The zip of this version |
| `babysitter-darwin-<arch>.dmg` | The same DMG. `releases/latest/download/babysitter-darwin-<arch>.dmg` always gives the latest one |
| `babysitter-darwin-<arch>.zip` | The same zip. The cask downloads it |

## Checks before the upload

`desktop` stops the release of an arch when one of these checks fails:

- `npm run make` wrote no app in `out/Babysitter-darwin-<arch>`.
- `codesign --verify --deep --strict` rejects the app.
- `lipo -archs` of the daemon in the app is not the arch of the runner.
- `babysitter --version` of the daemon in the app is not the tag.
- When the app has a Developer ID signature: `spctl --assess` rejects
  it, `xcrun stapler validate` finds no notarization ticket, or
  `Contents/Resources/app-update.yml` is missing.
- When the app has no Developer ID signature:
  `Contents/Resources/app-update.yml` is in the app.

`npm run make` gets three attempts, 30 seconds apart, because the
Apple notary service sometimes fails. The upload to the release gets
three attempts too.

## Signature

`frontend/scripts/mac-signing.ts` reads the environment and gives the
signature options to Forge:

| Variable | Result |
| --- | --- |
| `BABYSITTER_SKIP_SIGN=1` | No signature |
| `BABYSITTER_SIGN_IDENTITY` | Signs with that identity, with the hardened runtime. A failure stops the build |
| `APPLE_API_KEY`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER` | With a named identity, notarizes the app. `APPLE_API_KEY` is the path of the `.p8` file |
| `BABYSITTER_ADHOC_SIGN=1` | Ad hoc signature, with no hardened runtime. The app starts on arm64, but Gatekeeper does not accept it. A failure stops the build |
| none | Looks in the keychain for a `Developer ID Application` identity, and packages with no signature when it finds none |

When the repository has no `MAC_CERT_P12_BASE64` secret, `desktop` sets
`BABYSITTER_ADHOC_SIGN=1`. The cask then shows the command that removes
the quarantine.

### Turn on the Developer ID signature

You need a membership of the Apple Developer Program.

1. In Xcode, or at developer.apple.com, make a certificate of type
   **Developer ID Application**. Export it with its private key from
   Keychain Access as a `.p12` file, with a password.
2. At App Store Connect > Users and Access > Integrations, make an API
   key with the **Developer** role. Download the `.p8` file. Keep the key
   ID and the issuer ID.
3. Add these secrets to the repository:

| Secret | Value |
| --- | --- |
| `MAC_CERT_P12_BASE64` | `base64 -i certificate.p12` |
| `MAC_CERT_PASSWORD` | The password of the `.p12` file |
| `MAC_SIGN_IDENTITY` | The name of the identity, such as `Developer ID Application: Your Name (TEAMID)` |
| `APPLE_API_KEY_BASE64` | `base64 -i AuthKey_<id>.p8` |
| `APPLE_API_KEY_ID` | The key ID |
| `APPLE_API_ISSUER` | The issuer ID |

You do not have to change code. The next tag signs and notarizes the
app, and the cask no longer shows the quarantine command. Only the app
is notarized: the DMG itself has no signature, and Gatekeeper checks the
app that you copy out of it.

## Homebrew tap

`homebrew` runs only when the repository variable `HOMEBREW_TAP` is set.

1. Make a public repository named `homebrew-tap`, for example
   `deividfortuna/homebrew-tap`. Homebrew removes the `homebrew-` prefix,
   so users type `brew install deividfortuna/tap/babysitter`.
2. Make a fine-grained personal access token that can write the
   **Contents** of that repository only.
3. Add the repository variable `HOMEBREW_TAP` with the value
   `deividfortuna/homebrew-tap`, and the secret `HOMEBREW_TAP_TOKEN`
   with the token.

`frontend/scripts/write-cask.mjs` writes the cask from the checksums
of the two zips. The cask installs `Babysitter.app` and links
`Contents/Resources/daemon/babysitter` as the `babysitter` CLI. `brew
uninstall --zap babysitter` also removes the service, the database and
the logs.

The cask downloads the zip from the GitHub release. While
`deividfortuna/babysitter` is a private repository, the download fails
for everybody else. Make the repository public before you set
`HOMEBREW_TAP`, or the cask cannot install.

## Updates of the app

The app uses `electron-updater`. Once an hour it reads the GitHub
releases of `deividfortuna/babysitter`, downloads a newer zip, and asks
the user to restart. Settings > Updates has the switch for the
download and the channel, Stable or Prerelease.

Three things must be true, or the app does not update:

1. The repository is public. The app has no token. While the repository
   is private, each check gets a 404. The app writes it to the log and
   shows nothing.
2. The app has a Developer ID signature. Only then does Forge put
   `frontend/assets/app-update.yml` in the app (see `updateResources`
   in `frontend/scripts/mac-signing.ts`). An ad hoc app has no such file
   and says in Settings that it does not update itself, because
   Squirrel.Mac refuses to install over an ad hoc signature.
3. The release has `latest-mac.yml`. `feed` writes it with
   `frontend/scripts/write-feed.mjs`, only when the app is signed.

A new certificate from a different team breaks the updates of the apps
that users have. Squirrel.Mac accepts a new version only when its
signature satisfies the designated requirement of the old one.

`feed` runs after both arches of `desktop`. When one arch fails, the
release has no feed, and the apps that users have do not see it.

### Test an update on one Mac

You need the Developer ID identity in your keychain. Run from
`frontend/`:

```sh
npm version 0.9.0 --no-git-tag-version
BABYSITTER_SIGN_IDENTITY="Developer ID Application: <name> (<team>)" npm run make -- --arch=arm64
cp -R out/Babysitter-darwin-arm64/Babysitter.app /Applications/

npm version 0.9.1 --no-git-tag-version
BABYSITTER_SIGN_IDENTITY="Developer ID Application: <name> (<team>)" npm run make -- --arch=arm64
mkdir -p feed
cp out/make/zip/darwin/arm64/Babysitter-darwin-arm64-0.9.1.zip feed/Babysitter-0.9.1-darwin-arm64.zip
node scripts/write-feed.mjs 0.9.1 feed/Babysitter-0.9.1-darwin-arm64.zip > feed/latest-mac.yml
npx http-server feed -p 8765
```

In a second terminal, start the old app with the local feed:

```sh
BABYSITTER_UPDATE_FEED_URL=http://127.0.0.1:8765 /Applications/Babysitter.app/Contents/MacOS/babysitter
```

Open Settings > Updates and click **Check for updates**. The card in
the sidebar shows the download, then **Restart to update**. After the
restart, Settings shows 0.9.1. Put `package.json` back to its version
before you commit.

## Not in this release

- There is no Linux or Windows build of the app. Linux has the CLI
  archives of GoReleaser.
