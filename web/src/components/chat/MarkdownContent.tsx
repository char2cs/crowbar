import type { BundledLanguage } from 'shiki'
import {
  CodeBlock,
  CodeBlockHeader,
  CodeBlockTitle,
  CodeBlockFilename,
  CodeBlockActions,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { cn } from '@/lib/utils'

// ── Segment parsing ───────────────────────────────────────────────────────────

interface CodeSeg { type: 'code'; lang: string; code: string }
interface TextSeg { type: 'text'; content: string }
type Segment = CodeSeg | TextSeg

function parseSegments(md: string): Segment[] {
  const segs: Segment[] = []
  const re = /```(\w*)\n?([\s\S]*?)```/g
  let cursor = 0
  let m: RegExpExecArray | null

  while ((m = re.exec(md)) !== null) {
    if (m.index > cursor) segs.push({ type: 'text', content: md.slice(cursor, m.index) })
    segs.push({ type: 'code', lang: m[1] || 'text', code: m[2].replace(/\n$/, '') })
    cursor = m.index + m[0].length
  }

  if (cursor < md.length) segs.push({ type: 'text', content: md.slice(cursor) })
  return segs
}

// ── Inline tokenisation ───────────────────────────────────────────────────────

type InlineToken =
  | { kind: 'bold';   text: string }
  | { kind: 'italic'; text: string }
  | { kind: 'code';   text: string }
  | { kind: 'plain';  text: string }

function tokenizeInline(raw: string): InlineToken[] {
  const tokens: InlineToken[] = []
  // Order matters: bold before italic so ** isn't misread as two *
  const re = /(\*\*(.+?)\*\*)|(`(.+?)`)|(\*(.+?)\*|_(.+?)_)/gs
  let cursor = 0
  let m: RegExpExecArray | null

  while ((m = re.exec(raw)) !== null) {
    if (m.index > cursor) tokens.push({ kind: 'plain', text: raw.slice(cursor, m.index) })
    if (m[1])      tokens.push({ kind: 'bold',   text: m[2] })
    else if (m[3]) tokens.push({ kind: 'code',   text: m[4] })
    else           tokens.push({ kind: 'italic', text: m[6] ?? m[7] ?? '' })
    cursor = m.index + m[0].length
  }

  if (cursor < raw.length) tokens.push({ kind: 'plain', text: raw.slice(cursor) })
  return tokens
}

function Inline({ raw }: { raw: string }) {
  return (
    <>
      {tokenizeInline(raw).map((t, i) => {
        const key = `${t.kind}:${t.text}:${i}`
        if (t.kind === 'bold')   return <strong key={key} className="font-semibold">{t.text}</strong>
        if (t.kind === 'italic') return <em key={key}>{t.text}</em>
        if (t.kind === 'code')   return (
          <code key={key} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.88em] text-primary/90">
            {t.text}
          </code>
        )
        return <span key={key}>{t.text}</span>
      })}
    </>
  )
}

// ── Block rendering ───────────────────────────────────────────────────────────

function BlockContent({ content }: { content: string }) {
  const lines = content.split('\n')
  const nodes: React.ReactNode[] = []
  let i = 0

  while (i < lines.length) {
    const line = lines[i]

    if (line.trim() === '') { i++; continue }

    // Heading
    const h = line.match(/^(#{1,6})\s+(.+)/)
    if (h) {
      const lvl = h[1].length
      const Tag = `h${lvl}` as 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6'
      nodes.push(
        <Tag key={i} className={cn('font-semibold', lvl === 1 && 'text-base mt-3 mb-1.5', lvl === 2 && 'text-[0.92em] mt-2.5 mb-1', lvl >= 3 && 'text-[0.875em] mt-2 mb-0.5')}>
          <Inline raw={h[2]} />
        </Tag>
      )
      i++; continue
    }

    // HR
    if (/^[-*]{3,}$/.test(line.trim())) {
      nodes.push(<hr key={i} className="my-3 border-border" />)
      i++; continue
    }

    // Unordered list — consume consecutive bullet lines
    if (/^[*\-+] /.test(line)) {
      const items: string[] = []
      while (i < lines.length && /^[*\-+] /.test(lines[i])) {
        items.push(lines[i].replace(/^[*\-+] /, ''))
        i++
      }
      nodes.push(
        <ul key={`ul${i}`} className="my-1.5 ml-4 list-disc space-y-0.5">
          {items.map((item, k) => <li key={k}><Inline raw={item} /></li>)}
        </ul>
      )
      continue
    }

    // Ordered list
    if (/^\d+[.)]\s/.test(line)) {
      const items: string[] = []
      while (i < lines.length && /^\d+[.)]\s/.test(lines[i])) {
        items.push(lines[i].replace(/^\d+[.)]\s/, ''))
        i++
      }
      nodes.push(
        <ol key={`ol${i}`} className="my-1.5 ml-4 list-decimal space-y-0.5">
          {items.map((item, k) => <li key={k}><Inline raw={item} /></li>)}
        </ol>
      )
      continue
    }

    // Paragraph — collect adjacent non-blank, non-special lines
    const paraLines: string[] = []
    while (
      i < lines.length &&
      lines[i].trim() !== '' &&
      !/^[*\-+] /.test(lines[i]) &&
      !/^\d+[.)]\s/.test(lines[i]) &&
      !/^#{1,6} /.test(lines[i]) &&
      !/^[-*]{3,}$/.test(lines[i].trim())
    ) {
      paraLines.push(lines[i])
      i++
    }

    if (paraLines.length > 0) {
      nodes.push(
        <p key={`p${i}`} className="leading-relaxed mb-1 last:mb-0">
          <Inline raw={paraLines.join('\n')} />
        </p>
      )
    }
  }

  return <>{nodes}</>
}

// ── Public component ──────────────────────────────────────────────────────────

export function MarkdownContent({ content, className }: { content: string; className?: string }) {
  const segments = parseSegments(content)

  return (
    <div className={cn('flex flex-col gap-2', className)}>
      {segments.map((seg, idx) => {
        if (seg.type === 'code') {
          return (
            <CodeBlock
              key={idx}
              code={seg.code}
              language={seg.lang as BundledLanguage}
              className="text-[12px]"
            >
              <CodeBlockHeader>
                <CodeBlockTitle>
                  <CodeBlockFilename>{seg.lang || 'code'}</CodeBlockFilename>
                </CodeBlockTitle>
                <CodeBlockActions>
                  <CodeBlockCopyButton />
                </CodeBlockActions>
              </CodeBlockHeader>
            </CodeBlock>
          )
        }
        return <BlockContent key={idx} content={seg.content} />
      })}
    </div>
  )
}
