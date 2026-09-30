const avatarPixels = "36";

function sizedAvatar(avatarUrl: string): string {
  const url = new URL(avatarUrl);
  url.searchParams.set("s", avatarPixels);
  return url.toString();
}

export function avatarSource(name: string, bot: boolean, avatarUrl?: string): string | undefined {
  if (avatarUrl) return sizedAvatar(avatarUrl);
  if (bot) return undefined;
  return `https://github.com/${name}.png?size=${avatarPixels}`;
}
