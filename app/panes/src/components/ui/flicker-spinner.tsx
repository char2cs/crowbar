import { useEffect, useRef, useState } from 'react'
import type React from 'react'
import { buildFlickerStrip } from '@/components/ui/flicker-strip'
import { cn } from '@/lib/utils'

const SPINNERS = Object.values(
  import.meta.glob('./spinners/*.svg', { eager: true, query: '?raw', import: 'default' }),
) as string[]

const strips = new Map<number, ReturnType<typeof buildFlickerStrip>>()
function stripFor(index: number): ReturnType<typeof buildFlickerStrip> {
  if (!strips.has(index)) strips.set(index, buildFlickerStrip(SPINNERS[index] ?? ''))
  return strips.get(index) ?? null
}

export function FlickerSpinner({
  className,
  ...props
}: React.ComponentProps<'span'>): React.ReactElement {
  const [index] = useState(() => Math.floor(Math.random() * SPINNERS.length))
  const strip = stripFor(index)
  const boxRef = useRef<HTMLSpanElement>(null)
  const stripRef = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    const box = boxRef.current
    if (!box || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        const el = stripRef.current
        if (el) el.style.animationPlayState = entry.isIntersecting ? 'running' : 'paused'
      }
    })
    observer.observe(box)
    return () => observer.disconnect()
  }, [])

  return (
    <span
      ref={boxRef}
      role="status"
      aria-label="Loading"
      data-flicker-spinner=""
      data-essential-motion=""
      className={cn(
        'relative inline-flex size-4 shrink-0 items-center justify-center overflow-hidden',
        className,
      )}
      {...props}
    >
      {strip ? (
        <span
          ref={stripRef}
          aria-hidden="true"
          className="absolute inset-y-0 left-0 [&>svg]:block [&>svg]:size-full [&>svg]:mx-0!"
          style={{
            width: `${strip.frames * 100}%`,
            animation: `flicker-strip ${strip.duration} steps(${strip.frames}) infinite`,
            // react-doctor-disable-next-line no-permanent-will-change -- the rule's fix (add on animationstart, remove on animationend) does not apply: this animation is `infinite` and runs for the element's entire mounted lifetime, and the element is mounted only while something is loading. There is no "end" to remove the hint at, and dropping it costs the promotion this spinner was rebuilt from SMIL to get (−59% main thread, measured).
            willChange: 'transform',
          }}
          // react-doctor-disable-next-line dangerous-html-sink -- `markup` is derived from a static build-time SVG asset from ./spinners/*.svg (import.meta.glob '?raw'), never attacker-influenceable. DOMPurify would strip the load-bearing currentColor markup.
          dangerouslySetInnerHTML={{ __html: strip.markup }}
        />
      ) : (
        <span
          aria-hidden="true"
          className="size-full [&>svg]:size-full! [&>svg]:mx-0!"
          // react-doctor-disable-next-line dangerous-html-sink -- static build-time SVG asset, as above.
          dangerouslySetInnerHTML={{ __html: SPINNERS[index] ?? '' }}
        />
      )}
    </span>
  )
}
