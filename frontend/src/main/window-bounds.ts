export type Rect = { x: number; y: number; width: number; height: number };
export type Size = { width: number; height: number };

export function centeredBounds(workArea: Rect, preferred: Size): Rect {
  const width = Math.min(preferred.width, workArea.width);
  const height = Math.min(preferred.height, workArea.height);
  return {
    x: workArea.x + Math.round((workArea.width - width) / 2),
    y: workArea.y + Math.round((workArea.height - height) / 2),
    width,
    height,
  };
}
