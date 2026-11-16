const WORD_STAGGER_MS = 30
const MAX_STAGGER_MS = 320

/** The fade-in delay of word `index` among `total`: the step scales down for a long chunk instead of the delay capping, so every word keeps a distinct delay and the cascade still ends within the cap. */
export function staggerDelay(index: number, total: number): number {
  const step = total <= 1 ? 0 : Math.min(WORD_STAGGER_MS, MAX_STAGGER_MS / (total - 1))
  return index * step
}

/** Whitespace-preserving split: joining the result reconstructs the input. */
export function splitIntoWords(text: string): string[] {
  return text.match(/\s*\S+\s*/g) ?? [text]
}
