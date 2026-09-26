const EMPTY = "__default__";

export function toSelectValue(value: string): string {
  return value === "" ? EMPTY : value;
}

export function fromSelectValue(value: string): string {
  return value === EMPTY ? "" : value;
}
