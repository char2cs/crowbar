import { useEffect, useRef, useState } from 'react'
import type React from 'react'
import { buildFlickerStrip } from '@/components/ui/flicker-strip'
import { cn } from '@/lib/utils'

// Every ./spinners/*.svg as raw markup at build time (drop in a file to add one). Played as a baked
// stepped-transform strip, not SMIL (see flicker-strip.ts); inlined rather than <img> so the dots'
// fill="currentColor" inherits the theme token.
const SPINNERS = Object.values(
  import.meta.glob('./spinners/*.svg', { eager: true, query: '?raw', import: 'default' }),
) as string[]

// Baked lazily and memoised: an asset costs one parse the first time it is
// picked, and never again. Building all of them up front would hold ~30 strips
// in memory for the one or two a session actually shows.
const strips = new Map<number, ReturnType<typeof buildFlickerStrip>>()
function stripFor(index: number): ReturnType<typeof buildFlickerStrip> {
  if (!strips.has(index)) strips.set(index, buildFlickerStrip(SPINNERS[index] ?? ''))
  return strips.get(index) ?? null
}

export function FlickerSpinner({
  className,
  ...props
}: React.ComponentProps<'span'>): React.ReactElement {
  // Random-pick one spinner per instance, stable for the component's lifetime.
  const [index] = useState(() => Math.floor(Math.random() * SPINNERS.length))
  const strip = stripFor(index)
  const boxRef = useRef<HTMLSpanElement>(null)
  const stripRef = useRef<HTMLSpanElement>(null)

  // Park the animation while off-screen (the sidebar carousel, scrolled-out rows). Fail-open: only a
  // definitive not-intersecting pauses it, since a wrongly frozen spinner reads as "hung".
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
      // The stable hook for "this is the real flicker spinner" — call-site
      // tests assert on it. They used to look for an <animate> element, which
      // is no longer how the animation is expressed.
      data-flicker-spinner=""
      // Exempt from index.css's reduced-motion kill-switch: a frozen status spinner reads as
      // "hung". Guarded by __tests__/reduced-motion.test.ts.
      data-essential-motion=""
      // Sized by className and coloured by an ancestor text-* token (fill="currentColor").
      // overflow-hidden is the strip's frame window; shrink-0 stops a squeezed flex row narrowing the
      // box, which would play crushed slivers.
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
          // `mx-0!` cancels Button/Tabs' `[&_svg]:-mx-0.5`, which has no :not() opt-out and would
          // slide a sliver of the next frame into the window.
          className="absolute inset-y-0 left-0 [&>svg]:block [&>svg]:size-full [&>svg]:mx-0!"
          style={{
            // N frames wide; translateX(-100%) is the strip width, so steps(N) lands on frame
            // boundaries, and jump-end holds frame 0 from t=0 like SMIL.
            width: `${strip.frames * 100}%`,
            animation: `flicker-strip ${strip.duration} steps(${strip.frames}) infinite`,
            // Promoted, so playback is a compositor transform on a painted layer: no style recalc
            // or layer-tree rebuild, and it runs while the window is unfocused.
            // react-doctor-disable-next-line no-permanent-will-change -- the rule's fix (add on animationstart, remove on animationend) does not apply: this animation is `infinite` and runs for the element's entire mounted lifetime, and the element is mounted only while something is loading. There is no "end" to remove the hint at, and dropping it costs the promotion this spinner was rebuilt from SMIL to get (−59% main thread, measured).
            willChange: 'transform',
          }}
          // react-doctor-disable-next-line dangerous-html-sink -- `markup` is derived from a static build-time SVG asset from ./spinners/*.svg (import.meta.glob '?raw'), never attacker-influenceable. DOMPurify would strip the load-bearing currentColor markup.
          dangerouslySetInnerHTML={{ __html: strip.markup }}
        />
      ) : (
        // An asset the baker cannot reproduce exactly (see flicker-strip.ts)
        // falls back to inlining it as-is and letting its own SMIL run. Slower,
        // never wrong.
        <span
          aria-hidden="true"
          // This path inlines the asset verbatim, so there is no baked
          // `class="size-full"` to make a host's `:not([class*='size-'])` sizing
          // rule stand down — hence the `!` here rather than the opt-out.
          className="size-full [&>svg]:size-full! [&>svg]:mx-0!"
          // react-doctor-disable-next-line dangerous-html-sink -- static build-time SVG asset, as above.
          dangerouslySetInnerHTML={{ __html: SPINNERS[index] ?? '' }}
        />
      )}
    </span>
  )
}
