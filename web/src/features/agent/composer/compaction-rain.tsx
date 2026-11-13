import type { CSSProperties } from 'react'

const CELL = 6
const ROWS = 14
const PATTERN_HEIGHT = CELL * ROWS
// Tall enough to cover a pill grown to half the viewport once the pattern is dropped by a cell.
const LAYER_HEIGHT = 640
const HEAD_OPACITY = 0.34

interface Dot {
  cx: number
  cy: number
  opacity: number
}

interface Layer {
  id: string
  /** Seconds per full fall; the two layers differ so the columns never line up. */
  seconds: number
  dots: Dot[]
}

/** mulberry32: a tiny seeded generator, so the same width always rains the same columns. */
function seeded(seed: number): () => number {
  let a = seed
  return () => {
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const layouts = new Map<number, Layer[]>()

function layoutFor(width: number): Layer[] {
  const cached = layouts.get(width)
  if (cached) return cached
  const rnd = seeded(width)
  const int = (lo: number, hi: number) => lo + Math.floor(rnd() * (hi - lo + 1))
  const cols = Math.floor(width / CELL)
  const specs = [
    { count: Math.floor((cols * 2) / 5), seconds: 2.8 },
    { count: Math.floor(cols / 3), seconds: 4.2 },
  ]
  const layers = specs.map(({ count, seconds }, k) => {
    const free = Array.from({ length: cols }, (_, c) => c)
    const dots: Dot[] = []
    for (let n = 0; n < count; n++) {
      const column = free.splice(int(0, free.length - 1), 1)[0]
      const top = int(0, ROWS - 1)
      const length = int(3, 5)
      for (let s = 0; s < length; s++) {
        const head = s === length - 1
        dots.push({
          cx: column * CELL + CELL / 2,
          cy: ((top + s) % ROWS) * CELL + CELL / 2,
          opacity: head ? HEAD_OPACITY : Number((0.07 + (0.17 * s) / length).toFixed(2)),
        })
      }
    }
    return { id: `rain-${width}-${k}`, seconds, dots }
  })
  layouts.set(width, layers)
  return layers
}

/** Faint falling dot-matrix streaks behind the pill while the conversation compacts: two
 *  patterns, each moved by one stepped transform, so it stays cheap and the text stays readable. */
export function CompactionRain({ width }: { width: number }) {
  const layers = layoutFor(width)
  return (
    <svg className="rain" width="100%" height="100%" aria-hidden="true">
      <defs>
        {layers.map((layer) => (
          <pattern
            key={layer.id}
            id={layer.id}
            width={width}
            height={PATTERN_HEIGHT}
            patternUnits="userSpaceOnUse"
          >
            {layer.dots.map((dot) => (
              <circle
                key={`${dot.cx}-${dot.cy}`}
                cx={dot.cx}
                cy={dot.cy}
                r={1.4}
                opacity={dot.opacity}
              />
            ))}
          </pattern>
        ))}
      </defs>
      {layers.map((layer) => (
        <g key={layer.id} className="fall" style={{ '--t': `${layer.seconds}s` } as CSSProperties}>
          <rect
            y={-PATTERN_HEIGHT}
            width={width}
            height={LAYER_HEIGHT}
            fill={`url(#${layer.id})`}
          />
        </g>
      ))}
    </svg>
  )
}
