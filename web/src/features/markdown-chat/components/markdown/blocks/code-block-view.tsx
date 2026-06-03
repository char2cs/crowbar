import { useEffect, useState } from 'react'
import type { BlockViewProps } from '@/features/markdown-chat/lib/block-registry'

/**
 * Default renderer for fenced code blocks. Highlights `source` with Shiki,
 * which is dynamically imported so it stays out of the initial bundle. While
 * streaming (or before highlighting resolves) it renders plain text; on an
 * unknown language Shiki throws and we fall back to plain text too.
 */
export function CodeBlockView(props: BlockViewProps) {
  const { source, type, streaming } = props
  const lang = type || 'text'
  const [html, setHtml] = useState<string | null>(null)

  useEffect(() => {
    // Defer highlighting until the turn has finished streaming so we don't
    // re-tokenize incomplete, rapidly-changing source.
    if (streaming) {
      setHtml(null)
      return
    }

    let cancelled = false
    void (async () => {
      try {
        const { codeToHtml } = await import('shiki')
        const out = await codeToHtml(source, {
          lang,
          theme: 'github-dark-default',
        })
        if (!cancelled) setHtml(out)
      } catch {
        // Unknown language (or any Shiki failure): fall back to plain text.
        if (!cancelled) setHtml(null)
      }
    })()

    return () => {
      cancelled = true
    }
  }, [source, lang, streaming])

  const containerClass =
    'rounded-lg border border-border overflow-auto text-sm p-3 ' +
    '[&_pre.shiki]:!bg-transparent [&_pre.shiki]:m-0 [&_pre.shiki]:p-0'

  if (!html || streaming) {
    return (
      <div
        className={containerClass}
        style={{
          background: 'var(--code-highlight)',
          fontFamily: 'var(--font-editor, monospace)',
        }}
      >
        <pre className="m-0 p-0">
          <code>{source}</code>
        </pre>
      </div>
    )
  }

  return (
    <div
      className={containerClass}
      style={{
        background: 'var(--code-highlight)',
        fontFamily: 'var(--font-editor, monospace)',
      }}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
