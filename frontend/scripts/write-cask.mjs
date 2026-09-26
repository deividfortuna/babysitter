import { renderCask } from "./homebrew-cask.ts";

const [version, sha256Arm, sha256Intel, signature] = process.argv.slice(2);

if (signature !== "signed" && signature !== "unsigned") {
  console.error("usage: write-cask.mjs <version> <sha256 arm64> <sha256 x64> <signed|unsigned>");
  process.exit(2);
}

process.stdout.write(renderCask({ version, sha256Arm, sha256Intel, signed: signature === "signed" }));
