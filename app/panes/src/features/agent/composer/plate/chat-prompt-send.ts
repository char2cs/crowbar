import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'

/** Re-parses spent per prompt; past this a prompt keeps its other escapes. */
const PARSE_BUDGET = 16
/** Above this a prompt goes as serialized: one parse of it costs too much. */
const LENGTH_CAP = 2000

let last = { draft: '', sent: '' }

/** The draft's markdown as sent: a remark escape stays only if dropping it would change what a
 *  re-parse gives back (the bubble, queue and history all re-parse it). */
export function chatPromptToSend(draft: string): string {
  if (last.draft !== draft) last = { draft, sent: withoutNeedlessEscapes(draft) }
  return last.sent
}

/** Where `md` holds a backslash escape: a `\` before ASCII punctuation. */
function escapesIn(md: string): number[] {
  const at: number[] = []
  for (let i = 0; i < md.length - 1; i++) {
    if (md[i] !== '\\' || !/[!-/:-@[-`{-~]/.test(md[i + 1] ?? '')) continue
    at.push(i)
    i++
  }
  return at
}

/** Halves are tried before singles, so a prompt of needless escapes costs two parses. */
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
