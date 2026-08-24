/**
 * The word beside the spinner while a turn runs.
 *
 * CROWBAR'S OWN, and deliberately so. The provider reports no verb on any
 * channel — a CLI's spinner word is painted characters in its terminal and
 * reaches no hook — so this is flavour text, not telemetry. It says only what
 * `working` already says, in a way that reads as alive rather than hung.
 *
 * It must therefore never claim to be the provider talking: no provider name
 * beside it, and no word borrowed from one product's voice to describe another's
 * work.
 */
const VERBS = [
  'Working',
  'Thinking',
  'Digging',
  'Pondering',
  'Chewing',
  'Reasoning',
  'Considering',
  'Untangling',
  'Piecing it together',
  'Following the thread',
] as const

/** How long a verb holds before the next one, in ms. Slow enough to read. */
export const VERB_ROTATION_MS = 3_400

/**
 * The verb for a given tick, stable for a given (turn, tick) pair.
 *
 * Indexed rather than random so a re-render mid-tick never swaps the word under
 * someone's eyes, and so a test can assert the sequence.
 */
export function verbAt(tick: number): string {
  return VERBS[Math.abs(Math.floor(tick)) % VERBS.length] ?? VERBS[0]
}

export const VERB_COUNT = VERBS.length
