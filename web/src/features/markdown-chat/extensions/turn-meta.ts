import { EditorView } from '@codemirror/view'
import { getTurnRanges } from './turn-boundaries'
import type { MarkdownTurn } from '../types'

// Per-turn metadata (model + time sent) shown as a subtle line at the start of
// each turn, sticky-pinned to the top of the viewport while that turn is
// scrolled through. The next turn's line slides up and takes over.
//
// Why an overlay instead of a CM6 block widget: a sticky element is, by
// definition, always at the viewport edge, and CM6 forbids block decorations
// "around the viewport" from a field/plugin that updates. So we render the
// labels in our own absolutely-positioned layer and reproduce sticky behaviour
// from CM6's height map (works even for turns taller than the viewport).
//
// Space for the at-rest label is reserved by the `.cm-turn-head` padding added
// in turn-boundaries.ts, so the label never covers the turn's first line.

// Gap kept between the pinned label and the top of the viewport.
const STICKY_TOP = 10
// Fallback label height before the node has been measured.
const FALLBACK_LABEL_H = 20

function formatTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
}

export function metaLabel(turn: MarkdownTurn): string {
  const time = formatTime(turn.timestamp)
  if (turn.role === 'agent') {
    const model = turn.model || turn.authorName
    return [model, time].filter(Boolean).join(' · ')
  }
  return time
}

export interface TurnMetaHandle {
  refresh: () => void
  destroy: () => void
}

// Mounts the sticky metadata layer over `view` into `overlay`. Returns a handle
// whose `refresh` should be called on relevant editor updates (doc/geometry).
export function mountTurnMeta(
  view: EditorView,
  overlay: HTMLElement,
  getTurns: () => MarkdownTurn[],
): TurnMetaHandle {
  overlay.classList.add('cm-turn-meta-layer')
  const nodes = new Map<string, HTMLElement>()

  const build = () => {
    const ranges = getTurnRanges(view.state)
    const byId = new Map(getTurns().map((t) => [t.id, t]))
    const scrollTop = view.scrollDOM.scrollTop
    const viewH = view.scrollDOM.clientHeight
    const docLen = view.state.doc.length
    const seen = new Set<string>()

    for (const range of ranges) {
      const turn = byId.get(range.id)
      if (!turn) continue
      const label = metaLabel(turn)
      if (!label) continue

      // Viewport-relative band for this turn, from the height map (valid even
      // when the turn's ends are scrolled out of the rendered viewport).
      const turnTop = view.lineBlockAt(range.from).top - scrollTop
      const turnBottom =
        view.lineBlockAt(Math.min(range.to, docLen)).bottom - scrollTop
      if (turnBottom <= 0 || turnTop >= viewH) continue // fully off-screen

      seen.add(range.id)
      let el = nodes.get(range.id)
      if (!el) {
        el = document.createElement('div')
        el.className = `cm-turn-meta cm-turn-meta-${range.role}`
        const span = document.createElement('span')
        span.className = 'cm-turn-meta-text'
        el.appendChild(span)
        overlay.appendChild(el)
        nodes.set(range.id, el)
      }
      const span = el.firstChild as HTMLElement
      if (span.textContent !== label) span.textContent = label

      const labelH = el.offsetHeight || FALLBACK_LABEL_H
      // Pin within the band: rest at the turn's top gap, hold at STICKY_TOP
      // while scrolling through, then ride the turn's bottom out of view.
      const y = Math.min(Math.max(STICKY_TOP, turnTop), turnBottom - labelH)
      el.style.transform = `translateY(${y}px)`
    }

    for (const [id, el] of nodes) {
      if (!seen.has(id)) {
        el.remove()
        nodes.delete(id)
      }
    }
  }

  let frame = 0
  const schedule = () => {
    if (frame) return
    frame = requestAnimationFrame(() => {
      frame = 0
      build()
    })
  }

  view.scrollDOM.addEventListener('scroll', schedule, { passive: true })
  const ro = new ResizeObserver(schedule)
  ro.observe(view.scrollDOM)
  build()

  return {
    refresh: build,
    destroy: () => {
      if (frame) cancelAnimationFrame(frame)
      view.scrollDOM.removeEventListener('scroll', schedule)
      ro.disconnect()
      for (const el of nodes.values()) el.remove()
      nodes.clear()
    },
  }
}

// Same centered-column padding as .cm-line (turn-boundaries.ts / input).
const COLUMN_PAD = 'max(48px, calc((100% - 680px) / 2 + 48px))'

export const turnMetaTheme = EditorView.theme({
  '.cm-turn-meta-layer': {
    position: 'absolute',
    inset: '0',
    overflow: 'hidden',
    pointerEvents: 'none',
    zIndex: '5',
  },
  '.cm-turn-meta': {
    position: 'absolute',
    left: '0',
    right: '0',
    display: 'flex',
    justifyContent: 'flex-end',
    padding: `0 ${COLUMN_PAD}`,
    fontSize: '12px',
    lineHeight: '1.4',
    color: 'var(--muted-foreground)',
    // Padding-top inside the band gives a touch of breathing room above the text.
    paddingTop: '6px',
    // Will-change keeps transform updates cheap during scroll.
    willChange: 'transform',
  },
  '.cm-turn-meta-text': {
    fontVariantNumeric: 'tabular-nums',
    whiteSpace: 'nowrap',
  },
})
