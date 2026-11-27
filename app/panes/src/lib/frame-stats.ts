import { useSyncExternalStore } from 'react'

export interface FrameStats {
  fps: number
  maxDt: number
  drops: number
}

const WINDOW_MS = 500
const IDLE: FrameStats = { fps: 0, maxDt: 0, drops: 0 }

let stats = IDLE
let rafId = 0
const listeners = new Set<() => void>()

function startSampling() {
  let prev = performance.now()
  let windowStart = prev
  let count = 0
  let maxDt = 0
  let drops = 0

  const tick = (ts: number) => {
    const dt = ts - prev
    prev = ts
    if (dt > 1 && dt < 500) {
      count++
      if (dt > maxDt) maxDt = dt
      if (dt > 16) drops++
    }
    const elapsed = ts - windowStart
    if (elapsed >= WINDOW_MS) {
      stats = { fps: Math.round((count / elapsed) * 1000), maxDt: Math.round(maxDt), drops }
      count = 0
      maxDt = 0
      drops = 0
      windowStart = ts
      listeners.forEach((listener) => listener())
    }
    rafId = requestAnimationFrame(tick)
  }
  rafId = requestAnimationFrame(tick)
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  if (listeners.size === 1) startSampling()
  return () => {
    listeners.delete(listener)
    if (listeners.size > 0) return
    cancelAnimationFrame(rafId)
    stats = IDLE
  }
}

const subscribeNothing = () => () => {}
const getStats = () => stats
const getIdle = () => IDLE

export function useFrameStats(enabled: boolean): FrameStats {
  return useSyncExternalStore(enabled ? subscribe : subscribeNothing, enabled ? getStats : getIdle)
}
