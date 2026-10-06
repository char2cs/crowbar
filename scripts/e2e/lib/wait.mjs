// Every wait is a condition polled against a deadline, never a bare sleep.
// The poll cadence yields to the event loop only to let the observed system
// make progress; the verdict always comes from the condition itself.

const POLL_MS = 50

export async function until(describe, condition, { timeout = 15_000 } = {}) {
  const deadline = Date.now() + timeout
  let last
  for (;;) {
    try {
      last = await condition()
      if (last) return last
    } catch (err) {
      if (err?.name === 'Fatal') throw err
      last = err
    }
    if (Date.now() > deadline) {
      const detail = last instanceof Error ? `: ${last.message}` : ''
      throw new Error(`timed out after ${timeout}ms waiting for ${describe}${detail}`)
    }
    await new Promise((resolve) => setTimeout(resolve, POLL_MS))
  }
}

export function assert(condition, message) {
  if (!condition) throw new Error(`assertion failed: ${message}`)
}

export function assertEqual(actual, expected, message) {
  const a = JSON.stringify(actual)
  const e = JSON.stringify(expected)
  if (a !== e) throw new Error(`assertion failed: ${message}\n  expected ${e}\n  actual   ${a}`)
}
