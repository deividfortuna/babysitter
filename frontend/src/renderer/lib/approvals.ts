export function wholeNumber(field: string): number | undefined {
  const text = field.trim();
  if (text === "") return undefined;
  const parsed = Number(text);
  return Number.isInteger(parsed) ? parsed : undefined;
}

export function approvalsRequired(field: string): number | undefined {
  const parsed = wholeNumber(field);
  return parsed === undefined || parsed < 0 ? undefined : parsed;
}

export function approvalsInvalid(field: string): boolean {
  return field.trim() !== "" && approvalsRequired(field) === undefined;
}

export function approvalsField(value: number | null | undefined): string {
  return value == null ? "" : String(value);
}
