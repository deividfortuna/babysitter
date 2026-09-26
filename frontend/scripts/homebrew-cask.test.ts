import { expect, test } from "vitest";
import { renderCask } from "./homebrew-cask";

const release = {
  version: "0.2.0",
  sha256Arm: "a".repeat(64),
  sha256Intel: "b".repeat(64),
  signed: true,
};

test("the cask pins the version and the checksum of each arch", () => {
  const cask = renderCask(release);
  expect(cask).toContain('version "0.2.0"');
  expect(cask).toContain(`sha256 arm:   "${"a".repeat(64)}",\n         intel: "${"b".repeat(64)}"`);
});

test("the cask downloads the zip of the arch from the release of its version", () => {
  expect(renderCask(release)).toContain(
    'url "https://github.com/deividfortuna/babysitter/releases/download/v#{version}/babysitter-darwin-#{arch}.zip"',
  );
});

test("the cask installs the app and puts the bundled daemon on the PATH as the CLI", () => {
  const cask = renderCask(release);
  expect(cask).toContain('app "Babysitter.app"');
  expect(cask).toContain('binary "#{appdir}/Babysitter.app/Contents/Resources/daemon/babysitter"');
});

test("the cask requires the oldest macOS that both Electron and Go support", () => {
  expect(renderCask(release)).toContain("depends_on macos: :ventura");
});

test("a signed build needs no caveat", () => {
  expect(renderCask(release)).not.toContain("caveats");
});

test("an unsigned build keeps the quarantine and tells the user how to open it", () => {
  const cask = renderCask({ ...release, signed: false });
  expect(cask).not.toContain("flight");
  expect(cask).toContain("caveats <<~EOS");
  expect(cask).toContain("xattr -dr com.apple.quarantine #{appdir}/Babysitter.app");
});

test("zap removes the service, the data and the logs", () => {
  const cask = renderCask(release);
  expect(cask).toContain('launchctl: "com.deividfortuna.babysitter"');
  expect(cask).toContain('"~/Library/Application Support/babysitter"');
  expect(cask).toContain('"~/Library/LaunchAgents/com.deividfortuna.babysitter.plist"');
  expect(cask).toContain('"~/Library/Logs/babysitter"');
});

test("zap removes the downloads of the updater and the staging folder of Squirrel", () => {
  const cask = renderCask(release);
  expect(cask).toContain('"~/Library/Caches/babysitter-updater"');
  expect(cask).toContain('"~/Library/Caches/com.deividfortuna.babysitter.ShipIt"');
});

test("a signed build updates itself, so brew upgrade leaves it to the app", () => {
  expect(renderCask(release)).toContain("  auto_updates true\n");
});

test("an unsigned build cannot update itself, so brew upgrade keeps it current", () => {
  expect(renderCask({ ...release, signed: false })).not.toContain("auto_updates");
});

test.each([
  ["a version with a leading v", { version: "v0.2.0" }, 'version "v0.2.0" is not semver without a leading v'],
  ["a checksum that is not sha256", { sha256Arm: "abc" }, 'sha256 of arm64 "abc" is not 64 lower case hex digits'],
  ["an upper case checksum", { sha256Intel: "B".repeat(64) }, `sha256 of x64 "${"B".repeat(64)}"`],
])("refuses %s", (_, override, message) => {
  expect(() => renderCask({ ...release, ...override })).toThrow(message);
});
