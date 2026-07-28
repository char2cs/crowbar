import { isValidElement, useEffect, useRef, useState, type ReactNode } from 'react'
import DOMPurify from 'dompurify'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { MARKDOWN_PROSE_CLASS } from '@/features/panes/lib/markdown-prose'
import { cn } from '@/utils/cn'

// ── Shiki lazy singleton ──────────────────────────────────────────────────────

type HighlightFn = (code: string, lang: string) => Promise<string>

let highlightFnPromise: Promise<HighlightFn> | null = null

/**
 * Lazily initialise shiki on first use and cache the highlight function.
 * Returns null in environments where the dynamic import fails (SSR/jsdom).
 */
function getHighlightFn(): Promise<HighlightFn> | null {
  if (typeof window === 'undefined') return null // SSR guard

  if (!highlightFnPromise) {
    highlightFnPromise = import('shiki/bundle/full')
      .then(({ getSingletonHighlighter }) =>
        getSingletonHighlighter({ themes: ['github-dark'], langs: [] }).then((hl) => {
          return async (code: string, lang: string): Promise<string> => {
            // Dynamically load language on demand; ignore unknown languages.
            try {
              // loadLanguage is a no-op if already loaded.
              await hl.loadLanguage(lang as Parameters<typeof hl.loadLanguage>[0])
            } catch {
              // Unknown / unsupported language — fall through to plain render.
              return ''
            }
            return hl.codeToHtml(code, { lang, theme: 'github-dark' })
          }
        }),
      )
      .catch(() => {
        highlightFnPromise = null // allow retry on next render
        return async () => '' // fallback: no highlight
      })
  }

  return highlightFnPromise
}

// ── Highlighted code block ────────────────────────────────────────────────────

interface ShikiCodeProps {
  code: string
  lang: string
}

function ShikiCodeBlock({ code, lang }: ShikiCodeProps) {
  const [html, setHtml] = useState<string | null>(null)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    const promise = getHighlightFn()
    if (!promise) return

    promise
      .then((fn) => fn(code, lang))
      .then((result) => {
        // Shiki HTML-escapes all code text, but markdown here can carry
        // attacker-influenceable agent/PR content — DOMPurify.sanitize is
        // defense-in-depth guaranteeing no script/handler survives regardless
        // of shiki config (preserves shiki's span/class/style highlighting).
        if (mountedRef.current && result) setHtml(DOMPurify.sanitize(result))
      })
      .catch(() => {
        /* ignore — plain fallback stays visible */
      })

    return () => {
      mountedRef.current = false
    }
  }, [code, lang])

  if (html) {
    return (
      <div
        className="[&_pre]:!bg-transparent [&_pre]:!p-0 [&_.shiki]:overflow-x-auto [&_.shiki]:rounded-lg [&_.shiki]:bg-muted/60 [&_.shiki]:p-3 [&_.shiki]:text-xs"
        // biome-ignore lint/security/noDangerouslySetInnerHtml: shiki output is DOMPurify-sanitised
        // react-doctor-disable-next-line dangerous-html-sink -- `html` is shiki codeToHtml output (escapes all code text), additionally DOMPurify.sanitize'd above as defense-in-depth on attacker-influenceable agent content.
        dangerouslySetInnerHTML={{ __html: html }}
      />
    )
  }

  // Plain fallback while shiki loads or if it fails.
  return (
    <pre className="overflow-x-auto rounded-lg bg-muted/60 p-3 text-xs">
      <code>{code}</code>
    </pre>
  )
}

// ── Custom react-markdown components ─────────────────────────────────────────

interface CodeProps {
  className?: string
  children?: ReactNode
}

/** The text a react-markdown node renders, however deeply it is wrapped. */
function textOf(node: ReactNode): string {
  if (typeof node === 'string') return node
  if (typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textOf).join('')
  if (isValidElement<{ children?: ReactNode }>(node)) return textOf(node.props.children)
  return ''
}

/**
 * Fenced code blocks, claimed at the `pre` rather than at the `code`.
 *
 * The block/inline split used to be read from react-markdown's `inline` prop,
 * which v9 REMOVED — so it arrived as `undefined`, every code span took the
 * "not inline" branch, and every `` `path/to/file` `` in a review comment
 * rendered as a full-width grey block. In markdown a `pre` wraps a fenced block
 * and nothing else, so asking the `pre` is a question with an answer, and the
 * `code` override below is left to mean only what its name says.
 */
function MarkdownPre({ children }: { children?: ReactNode }) {
  const codeEl = isValidElement<CodeProps>(children) ? children : null
  const lang = /language-(\w+)/.exec(codeEl?.props.className ?? '')?.[1] ?? ''
  const code = textOf(codeEl ? codeEl.props.children : children).replace(/\n$/, '')

  if (lang) return <ShikiCodeBlock code={code} lang={lang} />

  // Fenced block without a language tag — plain pre/code.
  return (
    <pre className="overflow-x-auto rounded-lg bg-muted/60 p-3 text-xs">
      <code>{code}</code>
    </pre>
  )
}

function MarkdownInlineCode({ children }: CodeProps) {
  return <code className="rounded bg-muted/60 px-1 py-0.5 text-xs font-mono">{children}</code>
}

const MD_COMPONENTS = { code: MarkdownInlineCode, pre: MarkdownPre } as Parameters<
  typeof ReactMarkdown
>[0]['components']

// ── Public component ──────────────────────────────────────────────────────────

export function MarkdownPreview({ children, className }: { children: string; className?: string }) {
  return (
    <div className={cn(MARKDOWN_PROSE_CLASS, className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={MD_COMPONENTS}>
        {children}
      </ReactMarkdown>
    </div>
  )
}
