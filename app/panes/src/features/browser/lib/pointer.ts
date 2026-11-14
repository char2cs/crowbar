import { browserApi } from '@/lib/browser-bridge'
import { domAgentBody } from './dom-agent'

interface PointerItem {
  s: string
  tag: string
  role: string
  name: string
  text: string
  r: number[]
  h: string
  i: number
}

/** What the page returns for a finished pick; the tab id is added when it is written out. */
export interface PickResult {
  kind: 'element' | 'region' | 'text' | 'multi'
  title: string
  url: string
  items: PointerItem[]
  more: number
  box: number[]
  quote?: string
}

// The native eval caps a script at 60s; a pick that outlasts it ends as a failure.
const PICK_TIMEOUT_MS = 60_000
const CANCEL_TIMEOUT_MS = 5_000

/** Resolves when the user finishes the pick, or null when it is cancelled. */
export async function pickPointer(tabId: string): Promise<PickResult | null> {
  const raw = await browserApi.evaluate(tabId, domAgentBody('pick', {}), PICK_TIMEOUT_MS)
  return JSON.parse(raw) as PickResult | null
}

export function cancelPick(tabId: string): Promise<unknown> {
  return browserApi.evaluate(tabId, domAgentBody('cancelPick', {}), CANCEL_TIMEOUT_MS)
}

function label(result: PickResult): string {
  const [first] = result.items
  if (result.kind === 'text') return result.quote?.slice(0, 40) ?? ''
  return first?.name || first?.text || ''
}

function headline(result: PickResult, tab: string): string {
  const count = result.items.length + result.more
  const first = result.items[0]
  const kind = result.kind === 'element' ? ((first?.role || first?.tag) ?? 'element') : result.kind
  const tail = result.kind === 'element' ? '' : ` · ${count} item${count === 1 ? '' : 's'}`
  return `⌖ ${kind} ${JSON.stringify(label(result))} · ${result.url} · tab ${tab}${tail}`
}

/** The pointer text: readable lines, then one `⌖data` JSON line an agent can parse. */
export function formatPointer(result: PickResult, tab: string): string {
  const lines = [headline(result, tab)]
  if (result.kind !== 'element') {
    result.items.forEach((item, n) => {
      const what = item.name || item.text
      lines.push(` ${n + 1} ${item.role || item.tag}${what ? ` ${JSON.stringify(what)}` : ''}`)
    })
  }
  const data = {
    v: 1,
    tab,
    url: result.url,
    title: result.title,
    kind: result.kind,
    items: result.items,
    more: result.more,
    box: result.box,
    ...(result.quote ? { quote: result.quote } : {}),
  }
  lines.push(`⌖data ${JSON.stringify(data)}`)
  return lines.join('\n')
}
