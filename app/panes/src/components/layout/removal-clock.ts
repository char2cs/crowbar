export function secondsLeft(deadlineAt: number, now: number): number {
  return Math.max(0, Math.ceil((deadlineAt - now) / 1000))
}

export function startRemovalSecondsClock(deadlines: ReadonlyMap<string, number>): () => void {
  if (deadlines.size === 0) return () => {}
  let timer = 0
  const tick = () => {
    const now = Date.now()
    let delay = 1000
    for (const el of document.querySelectorAll<HTMLElement>('[data-removal-secs]')) {
      const deadlineAt = deadlines.get(el.dataset.removalSecs ?? '')
      if (deadlineAt === undefined) continue
      const secs = String(secondsLeft(deadlineAt, now))
      if (el.textContent !== secs) el.textContent = secs
      const left = deadlineAt - now
      if (left > 0) delay = Math.min(delay, ((left - 1) % 1000) + 1)
    }
    timer = window.setTimeout(tick, delay)
  }
  tick()
  return () => clearTimeout(timer)
}
