export function formatDuration(durationUs: number): string {
  return `${Number((durationUs / 1_000_000).toFixed(1))} 秒`;
}
