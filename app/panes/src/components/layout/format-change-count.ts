export function formatChangeCount(n: number): string {
  return n > 999 ? `${Math.round(n / 1000)}k` : `${n}`
}
