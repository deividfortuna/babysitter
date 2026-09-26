export type CaskRelease = {
  version: string;
  sha256Arm: string;
  sha256Intel: string;
  signed: boolean;
};

const VERSION = /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/;
const SHA256 = /^[0-9a-f]{64}$/;

function assertRelease({ version, sha256Arm, sha256Intel }: CaskRelease): void {
  if (!VERSION.test(version)) throw new Error(`version ${JSON.stringify(version)} is not semver without a leading v`);
  for (const [arch, sum] of [
    ["arm64", sha256Arm],
    ["x64", sha256Intel],
  ]) {
    if (!SHA256.test(sum)) throw new Error(`sha256 of ${arch} ${JSON.stringify(sum)} is not 64 lower case hex digits`);
  }
}

const UNSIGNED_CAVEAT = `
  caveats <<~EOS
    Babysitter has no Developer ID signature yet, so macOS blocks the first
    launch. Allow it in System Settings > Privacy & Security, or run:
      xattr -dr com.apple.quarantine #{appdir}/Babysitter.app
  EOS
`;

export function renderCask(release: CaskRelease): string {
  assertRelease(release);
  const { version, sha256Arm, sha256Intel, signed } = release;
  return `cask "babysitter" do
  arch arm: "arm64", intel: "x64"

  version "${version}"
  sha256 arm:   "${sha256Arm}",
         intel: "${sha256Intel}"

  url "https://github.com/deividfortuna/babysitter/releases/download/v#{version}/babysitter-darwin-#{arch}.zip"
  name "Babysitter"
  desc "Watches GitHub pull requests and has a coding agent fix them"
  homepage "https://github.com/deividfortuna/babysitter"

  livecheck do
    url :url
    strategy :github_latest
  end

${signed ? "  auto_updates true\n" : ""}  depends_on macos: :ventura

  app "Babysitter.app"
  binary "#{appdir}/Babysitter.app/Contents/Resources/daemon/babysitter"

  zap launchctl: "com.deividfortuna.babysitter",
      trash:     [
        "~/Library/Application Support/babysitter",
        "~/Library/Caches/babysitter-updater",
        "~/Library/Caches/com.deividfortuna.babysitter.ShipIt",
        "~/Library/LaunchAgents/com.deividfortuna.babysitter.plist",
        "~/Library/Logs/babysitter",
      ]
${signed ? "" : UNSIGNED_CAVEAT}end
`;
}
