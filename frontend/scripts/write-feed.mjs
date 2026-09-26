import { createHash } from "node:crypto";
import { createReadStream, statSync } from "node:fs";
import path from "node:path";
import { renderFeed } from "./update-feed.ts";

const [version, ...zips] = process.argv.slice(2);

if (!version || zips.length === 0) {
  console.error("usage: write-feed.mjs <version> <zip>...");
  process.exit(2);
}

async function describe(zip) {
  const hash = createHash("sha512");
  for await (const chunk of createReadStream(zip)) hash.update(chunk);
  return {
    name: path.basename(zip),
    sha512: hash.digest("base64"),
    size: statSync(zip).size,
  };
}

const files = await Promise.all(zips.map(describe));

process.stdout.write(renderFeed({ version, files, releaseDate: new Date().toISOString() }));
