const ACTIONS_PREFIX = ">";

export function paletteMode(query: string): "places" | "actions" {
  return query.startsWith(ACTIONS_PREFIX) ? "actions" : "places";
}

export function actionValue(label: string): string {
  return `${ACTIONS_PREFIX}${label}`;
}
