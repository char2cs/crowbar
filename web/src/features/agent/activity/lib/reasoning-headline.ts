/**
 * Splits the agent's live words into a one-line heading and the body below it.
 *
 * A leading bold span on its own line is the provider's own title for the thought;
 * otherwise the first line is the heading. The body is whatever is left, so the
 * heading text is never drawn twice.
 */
export function reasoningHeadline(text: string): { headline: string; body: string } {
  const plain = text.replace(/^\s+/, '')
  const title = /^\*{2}([^*\n]+)\*{2}[ \t]*(?:\n|$)/.exec(plain)
  if (title) {
    return { headline: title[1].trim(), body: strip(plain.slice(title[0].length)) }
  }
  const nl = plain.indexOf('\n')
  const first = nl === -1 ? plain : plain.slice(0, nl)
  return { headline: strip(first), body: nl === -1 ? '' : strip(plain.slice(nl + 1)) }
}

function strip(s: string): string {
  return s.replace(/\*{1,3}/g, '').trim()
}
