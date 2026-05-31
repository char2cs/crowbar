// web/src/features/markdown-chat/components/markdown-chat-toolbar.tsx
import { useState, useRef, useEffect } from 'react'
import type { EditorView } from '@codemirror/view'
import { nanoid } from 'nanoid'

const CODE_LANGUAGES = [
  'typescript', 'javascript', 'python', 'go', 'shell', 'json', 'plain',
] as const

type CodeLanguage = (typeof CODE_LANGUAGES)[number]

interface ToolbarProps {
  editorView: EditorView | null
  onInsertWidget: (widgetType: string, widgetId: string) => void
}

function wrapSelection(view: EditorView, syntax: string) {
  const { from, to } = view.state.selection.main
  const selected = view.state.sliceDoc(from, to)
  view.dispatch({
    changes: { from, to, insert: `${syntax}${selected}${syntax}` },
    selection: { anchor: from + syntax.length, head: to + syntax.length },
  })
  view.focus()
}

function prependLine(view: EditorView, prefix: string) {
  const { from } = view.state.selection.main
  const line = view.state.doc.lineAt(from)
  const already = line.text.startsWith(prefix)
  view.dispatch({
    changes: already
      ? { from: line.from, to: line.from + prefix.length, insert: '' }
      : { from: line.from, insert: prefix },
  })
  view.focus()
}

function insertBlock(view: EditorView, content: string) {
  const { from } = view.state.selection.main
  view.dispatch({ changes: { from, insert: `\n${content}\n` } })
  view.focus()
}

function insertExcalidraw(view: EditorView, widgetId: string) {
  insertBlock(view, `\`\`\`excalidraw widget-id:${widgetId}\n\`\`\``)
}

function insertCodeBlock(view: EditorView, lang: CodeLanguage) {
  insertBlock(view, `\`\`\`${lang}\n\n\`\`\``)
}

function insertMermaid(view: EditorView) {
  insertBlock(view, '```mermaid\nflowchart LR\n    A --> B\n```')
}

function ToolbarButton({
  children,
  onClick,
  title,
}: {
  children: React.ReactNode
  onClick: () => void
  title: string
}) {
  return (
    <button
      title={title}
      onClick={onClick}
      className="flex h-6 min-w-6 items-center justify-center rounded px-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    >
      {children}
    </button>
  )
}

function MenuItem({ children, onClick }: { children: React.ReactNode; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-foreground transition-colors hover:bg-muted"
    >
      {children}
    </button>
  )
}

function InsertDropdown({
  onInsertExcalidraw,
  onInsertCodeBlock,
  onInsertMermaid,
}: {
  onInsertExcalidraw: () => void
  onInsertCodeBlock: (lang: CodeLanguage) => void
  onInsertMermaid: () => void
}) {
  const [open, setOpen] = useState(false)
  const [codeOpen, setCodeOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) { setCodeOpen(false); return }
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div ref={ref} className="relative">
      <ToolbarButton title="Insert block" onClick={() => setOpen((v) => !v)}>
        + Insert ▾
      </ToolbarButton>
      {open && (
        <div className="absolute left-0 top-full z-50 mt-1 min-w-44 rounded-md border border-border bg-popover shadow-md">
          <MenuItem
            onClick={() => { onInsertExcalidraw(); setOpen(false) }}
          >
            ✏️ Excalidraw drawing
          </MenuItem>
          <div className="relative">
            <MenuItem onClick={() => setCodeOpen((v) => !v)}>
              <span className="font-mono">&lt;/&gt;</span> Code block ▸
            </MenuItem>
            {codeOpen && (
              <div className="absolute left-full top-0 min-w-36 rounded-md border border-border bg-popover shadow-md">
                {CODE_LANGUAGES.map((lang) => (
                  <MenuItem
                    key={lang}
                    onClick={() => {
                      onInsertCodeBlock(lang)
                      setOpen(false)
                      setCodeOpen(false)
                    }}
                  >
                    {lang}
                  </MenuItem>
                ))}
              </div>
            )}
          </div>
          <MenuItem onClick={() => { onInsertMermaid(); setOpen(false) }}>
            📊 Mermaid diagram
          </MenuItem>
        </div>
      )}
    </div>
  )
}

export function MarkdownChatToolbar({ editorView, onInsertWidget }: ToolbarProps) {
  const v = editorView

  return (
    <div className="flex shrink-0 items-center gap-0.5 border-t border-border px-2 py-1">
      {/* Formatting group */}
      <ToolbarButton title="Bold" onClick={() => v && wrapSelection(v, '**')}>
        <b>B</b>
      </ToolbarButton>
      <ToolbarButton title="Italic" onClick={() => v && wrapSelection(v, '*')}>
        <i>I</i>
      </ToolbarButton>
      <ToolbarButton title="Inline code" onClick={() => v && wrapSelection(v, '`')}>
        <span className="font-mono text-[10px]">`x`</span>
      </ToolbarButton>

      <div className="mx-1 h-4 w-px shrink-0 bg-border" />

      <ToolbarButton title="Heading 1" onClick={() => v && prependLine(v, '# ')}>
        H1
      </ToolbarButton>
      <ToolbarButton title="Heading 2" onClick={() => v && prependLine(v, '## ')}>
        H2
      </ToolbarButton>
      <ToolbarButton title="Heading 3" onClick={() => v && prependLine(v, '### ')}>
        H3
      </ToolbarButton>

      <div className="mx-1 h-4 w-px shrink-0 bg-border" />

      <InsertDropdown
        onInsertExcalidraw={() => {
          if (!v) return
          const id = nanoid()
          onInsertWidget('excalidraw', id)
          insertExcalidraw(v, id)
        }}
        onInsertCodeBlock={(lang) => v && insertCodeBlock(v, lang)}
        onInsertMermaid={() => v && insertMermaid(v)}
      />
    </div>
  )
}
