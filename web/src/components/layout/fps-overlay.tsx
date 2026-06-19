import { useEffect, useRef, useState } from 'react'
import { useSettingsStore } from '@/features/settings/store'
import { cn } from '@/utils/cn'

interface FrameStats {
  fps: number
  maxDt: number
  drops: number
}

function FpsOverlayInner() {
  const [stats, setStats] = useState<FrameStats>({ fps: 0, maxDt: 0, drops: 0 })

  const prevTsRef = useRef(performance.now())
  const countRef = useRef(0)
  const maxDtRef = useRef(0)
  const dropsRef = useRef(0)
  const windowStartRef = useRef(performance.now())

  useEffect(() => {
    let rafId: number

    function tick(ts: number) {
      const dt = ts - prevTsRef.current
      prevTsRef.current = ts

      // Ignore the first frame and any tab-backgrounding pauses
      if (dt > 1 && dt < 500) {
        countRef.current++
        if (dt > maxDtRef.current) maxDtRef.current = dt
        // A "drop" is any frame that took longer than one 60fps interval —
        // i.e. the compositor missed at least one 120fps slot.
        if (dt > 16) dropsRef.current++
      }

      const elapsed = ts - windowStartRef.current
      if (elapsed >= 500) {
        setStats({
          fps: Math.round((countRef.current / elapsed) * 1000),
          maxDt: Math.round(maxDtRef.current),
          drops: dropsRef.current,
        })
        countRef.current = 0
        maxDtRef.current = 0
        dropsRef.current = 0
        windowStartRef.current = ts
      }

      rafId = requestAnimationFrame(tick)
    }

    rafId = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(rafId)
  }, [])

  const { fps, maxDt, drops } = stats
  const fpsColor =
    fps >= 110 ? 'text-success' : fps >= 80 ? 'text-warning' : fps > 0 ? 'text-destructive' : 'text-muted-foreground'

  return (
    <div
      className="fixed bottom-8 right-3 z-[2147483647] pointer-events-none select-none rounded-md px-2.5 py-1.5 font-mono text-[11px] leading-none tabular-nums shadow-xl"
      style={{ background: 'rgba(0,0,0,0.72)', backdropFilter: 'blur(10px)' }}
      aria-hidden
    >
      <span className={cn('font-bold', fpsColor)}>{fps > 0 ? fps : '—'}</span>
      <span className="text-muted-foreground"> fps</span>
      <span className="mx-1.5 text-muted-foreground/30">·</span>
      <span className="text-muted-foreground">max </span>
      <span className="text-foreground">{maxDt > 0 ? `${maxDt}ms` : '—'}</span>
      <span className="mx-1.5 text-muted-foreground/30">·</span>
      <span className={drops > 0 ? 'text-destructive font-medium' : 'text-muted-foreground'}>
        {drops} {drops === 1 ? 'drop' : 'drops'}
      </span>
    </div>
  )
}

export function FpsOverlay() {
  const visible = useSettingsStore((s) => s.settings.showFpsOverlay)
  if (!visible) return null
  return <FpsOverlayInner />
}
