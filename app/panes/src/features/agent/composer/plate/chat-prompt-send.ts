import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'

const PARSE_BUDGET = 16
const LENGTH_CAP = 2000

let last = { draft: '', sent: '' }

export function chatPromptToSend(draft: string): string {
  if (last.draft !== draft) last = { draft, sent: withoutNeedlessEscapes(draft) }
  return last.sent
}

function escapesIn(md: string): number[] {
  const at: number[] = []
  for (let i = 0; i < md.length - 1; i++) {
    if (md[i] !== '\\' || !/[!-/:-@[-`{-~]/.test(md[i + 1] ?? '')) continue
    at.push(i)
    i++
  }
  return at
}

function withoutNeedlessEscapes(md: string): string {
  const escapes = md.length > LENGTH_CAP ? [] : escapesIn(md)
  if (escapes.length === 0) return md
  const read = (text: string) => JSON.stringify(chatMarkdownToValue(text))
  const target = read(md)
  const dropped = new Set<number>()
  const without = (extra: number[]) => {
    const skip = new Set([...dropped, ...extra])
    let out = ''
    for (let i = 0; i < md.length; i++) if (!skip.has(i)) out += md[i]
    return out
  }
  let budget = PARSE_BUDGET
  const tryDrop = (group: number[]) => {
    if (group.length === 0 || budget === 0) return
    budget--
    if (read(without(group)) === target) {
      for (const i of group) dropped.add(i)
      return
    }
    if (group.length === 1) return
    const half = Math.ceil(group.length / 2)
    tryDrop(group.slice(0, half))
    tryDrop(group.slice(half))
  }
  tryDrop(escapes)
  return without([])
}
