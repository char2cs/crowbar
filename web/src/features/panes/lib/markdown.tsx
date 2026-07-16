import { useEffect, useRef, useState } from 'react'
import DOMPurify from 'dompurify'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
// react-doctor-disable-next-line prefer-dynamic-import -- only imported by comment-composer.tsx, itself already behind the `GitDiffEditorStackLazy` React.lazy() boundary (review-diff-tab.tsx). Verified via `bunx vite build`: 0 "codemirror" occurrences in the entry chunk, 3 in the git-diff-editor-stack chunk.
import { cn } from '@/utils/cn'

/** Transparent CodeMirror theme so the editor blends into its container. */

/** Shared prose styling for rendered markdown across the branch-review feature. */
export const MARKDOWN_PROSE_CLASS =
  'prose prose-sm prose-invert max-w-none text-sm text-foreground ' +
  '[&_h1]:text-base [&_h1]:font-semibold [&_h1]:mb-2 [&_h1]:mt-3 ' +
  '[&_h2]:text-sm [&_h2]:font-semibold [&_h2]:mb-1.5 [&_h2]:mt-3 ' +
  '[&_h3]:text-sm [&_h3]:font-medium [&_h3]:mb-1 [&_h3]:mt-2 ' +
  '[&_p]:mb-2 [&_p]:leading-relaxed ' +
  '[&_ul]:my-1.5 [&_ul]:pl-4 [&_li]:my-0.5 ' +
  '[&_ol]:my-1.5 [&_ol]:pl-4 ' +
  '[&_code]:rounded [&_code]:bg-muted/60 [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-xs [&_code]:font-mono ' +
  '[&_pre]:rounded-lg [&_pre]:bg-muted/60 [&_pre]:p-3 [&_pre]:text-xs [&_pre]:overflow-x-auto ' +
  '[&_pre_code]:bg-transparent [&_pre_code]:p-0 ' +
  '[&_strong]:font-semibold [&_strong]:text-foreground ' +
  '[&_em]:italic ' +
  '[&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground ' +
  '[&_hr]:border-border [&_hr]:my-3 ' +
  '[&_a]:text-primary [&_a]:underline-offset-2 [&_a]:hover:underline'

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
  inline?: boolean
  className?: string
  children?: React.ReactNode
}

function MarkdownCode({ inline, className, children }: CodeProps) {
  const match = /language-(\w+)/.exec(className ?? '')
  const lang = match?.[1] ?? ''
  const code = String(children ?? '').replace(/\n$/, '')

  if (!inline && lang) {
    return <ShikiCodeBlock code={code} lang={lang} />
  }

  if (!inline && !lang) {
    // Fenced block without a language tag — plain pre/code.
    return (
      <pre className="overflow-x-auto rounded-lg bg-muted/60 p-3 text-xs">
        <code>{code}</code>
      </pre>
    )
  }

  // Inline code.
  return <code className="rounded bg-muted/60 px-1 py-0.5 text-xs font-mono">{children}</code>
}

const MD_COMPONENTS = { code: MarkdownCode } as Parameters<typeof ReactMarkdown>[0]['components']

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
