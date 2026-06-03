/* eslint-disable @typescript-eslint/no-explicit-any */
import ReactMarkdown from 'react-markdown'
import type { Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { visit } from 'unist-util-visit'
import type { ToolCallData, WidgetData } from '../../types'
import { MarkdownBlock } from './markdown-block'
import { ToolCallPill } from '../tool-call-pill'

interface TurnMarkdownProps {
  content: string
  widgets?: WidgetData[]
  streaming?: boolean
  onWidgetChange?: (widgetId: string, payload: unknown) => void
}

// remark plugin: carry a fenced block's language + meta onto the resulting
// <code> element as data-* attributes, so the `code` renderer can reconstruct
// the full info string (e.g. "excalidraw widget-id:abc"). mdast drops `meta`
// otherwise.
function remarkFenceMeta() {
  return (tree: any) => {
    visit(tree, 'code', (node: any) => {
      node.data = node.data || {}
      node.data.hProperties = {
        ...(node.data.hProperties || {}),
        'data-lang': node.lang || '',
        'data-meta': node.meta || '',
      }
    })
  }
}

// remark plugin: turn `<!-- tool-call:{json} -->` comment lines into a custom
// <toolcall> element (otherwise react-markdown drops HTML comments).
function remarkToolCall() {
  return (tree: any) => {
    visit(tree, 'html', (node: any, index: number | undefined, parent: any) => {
      if (index == null || !parent) return
      const m = node.value.match(/<!--\s*tool-call:([\s\S]+?)\s*-->/)
      if (!m) return
      parent.children[index] = {
        type: 'toolCall',
        children: [],
        data: { hName: 'toolcall', hProperties: { payload: m[1] } },
      }
    })
  }
}

const remarkPlugins = [remarkGfm, remarkFenceMeta, remarkToolCall]

function buildComponents(props: TurnMarkdownProps): Components {
  const { widgets, streaming, onWidgetChange } = props

  const components: Record<string, any> = {
    // Fenced code is <pre><code>; unwrap the <pre> so the block provides its
    // own container, and dispatch the <code> to the block registry.
    pre: ({ children }: any) => <>{children}</>,
    code: ({ node, className, children }: any) => {
      const dataLang =
        node?.properties?.dataLang ?? node?.properties?.['data-lang']
      const dataMeta =
        node?.properties?.dataMeta ?? node?.properties?.['data-meta']
      const langFromClass = /language-(\w+)/.exec(className || '')?.[1] ?? ''
      const isFenced = dataLang !== undefined || langFromClass !== ''

      if (!isFenced) {
        return (
          <code
            className="rounded px-1 py-0.5 text-[0.9em]"
            style={{
              fontFamily: 'var(--font-editor, monospace)',
              background: 'var(--code-highlight)',
            }}
          >
            {children}
          </code>
        )
      }

      const info = [dataLang || langFromClass, dataMeta]
        .filter(Boolean)
        .join(' ')
      const source = String(children ?? '').replace(/\n$/, '')
      return (
        <MarkdownBlock
          info={info}
          source={source}
          widgets={widgets}
          streaming={streaming}
          onChange={onWidgetChange}
        />
      )
    },
    toolcall: ({ payload }: any) => {
      try {
        return <ToolCallPill data={JSON.parse(payload) as ToolCallData} />
      } catch {
        return null
      }
    },
    h1: ({ children }: any) => (
      <h1 className="mt-6 mb-2 font-bold" style={{ fontSize: '1.5em', lineHeight: 1.3 }}>
        {children}
      </h1>
    ),
    h2: ({ children }: any) => (
      <h2 className="mt-6 mb-2 font-bold" style={{ fontSize: '1.25em', lineHeight: 1.3 }}>
        {children}
      </h2>
    ),
    h3: ({ children }: any) => (
      <h3 className="mt-5 mb-2 font-semibold" style={{ fontSize: '1.1em', lineHeight: 1.3 }}>
        {children}
      </h3>
    ),
    p: ({ children }: any) => <p className="my-3 leading-[1.75]">{children}</p>,
    ul: ({ children }: any) => <ul className="my-3 list-disc pl-6">{children}</ul>,
    ol: ({ children }: any) => <ol className="my-3 list-decimal pl-6">{children}</ol>,
    li: ({ children }: any) => <li className="my-1 leading-[1.75]">{children}</li>,
    a: ({ children, href }: any) => (
      <a className="underline underline-offset-2" href={href} target="_blank" rel="noreferrer">
        {children}
      </a>
    ),
    blockquote: ({ children }: any) => (
      <blockquote className="my-3 border-l-2 border-border pl-3 text-muted-foreground">
        {children}
      </blockquote>
    ),
    hr: () => <hr className="my-4 border-border" />,
    strong: ({ children }: any) => <strong className="font-bold">{children}</strong>,
    em: ({ children }: any) => <em className="italic">{children}</em>,
  }

  return components as Components
}

// Renders one turn's markdown to React. While the turn is still streaming, it
// renders plain text + a cursor (cheap) and defers the full markdown parse until
// the turn finalizes — avoiding a full re-parse on every streamed token.
export function TurnMarkdown(props: TurnMarkdownProps) {
  const { content, streaming } = props

  if (streaming) {
    return (
      <div className="whitespace-pre-wrap leading-[1.75]">
        {content}
        <span
          className="ml-0.5 inline-block h-[1em] w-[2px] translate-y-[2px] animate-pulse"
          style={{ background: 'var(--foreground)' }}
        />
      </div>
    )
  }

  return (
    // First block: no top margin (aligns text top with the margin label).
    // Last block: no bottom margin (so a tinted user turn wraps tightly, no gap).
    <div className="md-body [&>*:first-child]:mt-0 [&>*:last-child]:mb-0">
      <ReactMarkdown remarkPlugins={remarkPlugins} components={buildComponents(props)}>
        {content}
      </ReactMarkdown>
    </div>
  )
}
