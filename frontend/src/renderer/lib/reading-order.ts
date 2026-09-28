const GENERATED = [
  /(^|\/)(package-lock\.json|npm-shrinkwrap\.json|yarn\.lock|pnpm-lock\.yaml|bun\.lockb?|go\.sum|Cargo\.lock|poetry\.lock|uv\.lock|Gemfile\.lock|composer\.lock|Podfile\.lock|flake\.lock)$/,
  /\.min\.(js|css)$/,
  /\.(pb|gen|generated)\.[a-z]+$/,
  /(^|\/)(__generated__|generated|vendor|dist|node_modules)\//,
  /\.snap$/,
];

const TEST = [/(^|\/)(__tests__|tests?|spec)\//, /[._-](test|spec)\.[^/]+$/, /_test\.go$/, /(^|\/)test_[^/]+\.py$/];

const TEST_FOLDER = /(^|\/)(__tests__|tests?|spec)\//g;

const TEST_MARK = /[._-](test|spec)$/;

export function isGenerated(path: string): boolean {
  return GENERATED.some((pattern) => pattern.test(path));
}

export function isTest(path: string): boolean {
  return TEST.some((pattern) => pattern.test(path));
}

function subjectOf(path: string): string {
  const slash = path.lastIndexOf("/");
  const folder = path.slice(0, slash + 1).replace(TEST_FOLDER, "$1");
  const stem = path.slice(slash + 1).split(".")[0];
  return folder + stem.replace(/^test_/, "").replace(TEST_MARK, "");
}

function byPath(a: { path: string }, b: { path: string }): number {
  if (a.path === b.path) return 0;
  return a.path < b.path ? -1 : 1;
}

export function readingOrder<T extends { path: string }>(files: readonly T[]): T[] {
  const generated = files.filter((f) => isGenerated(f.path)).sort(byPath);
  const tests = files.filter((f) => !isGenerated(f.path) && isTest(f.path));
  const sources = files.filter((f) => !isGenerated(f.path) && !isTest(f.path)).sort(byPath);
  const position = new Map(sources.map((f, i) => [subjectOf(f.path), i]));
  const rank = (f: T) => position.get(subjectOf(f.path)) ?? sources.length;
  tests.sort((a, b) => rank(a) - rank(b) || byPath(a, b));
  return [...sources, ...tests, ...generated];
}
