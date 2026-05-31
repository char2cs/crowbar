import type { EditorView } from '@codemirror/view'
import { nanoid } from 'nanoid'
import { SendHorizontal, Square, Pencil, Code2, ChartNetwork } from 'lucide-react'
import {
  Toolbar,
  ToolbarButton,
  ToolbarGroup,
  ToolbarSeparator,
} from '@/components/ui/toolbar'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
} from '@/components/ui/dropdown-menu'

const CODE_LANGUAGES = [
  'typescript', 'javascript', 'python', 'go', 'shell', 'json', 'plain',
] as const

type CodeLanguage = (typeof CODE_LANGUAGES)[number]

interface ToolbarProps {
  editorView: EditorView | null
  onInsertWidget: (widgetType: string, widgetId: string) => void
  onSubmit: () => void
  isStreaming?: boolean
  onStop?: () => void
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
  const pos = view.state.doc.length
  view.dispatch({ changes: { from: pos, insert: `\n${content}\n` } })
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

function InsertDropdown({
  onInsertExcalidraw,
  onInsertCodeBlock,
  onInsertMermaid,
}: {
  onInsertExcalidraw: () => void
  onInsertCodeBlock: (lang: CodeLanguage) => void
  onInsertMermaid: () => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="ghost" size="sm" className="h-6 px-2 text-xs text-muted-foreground" />
        }
      >
        + Insert
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" className="min-w-44">
        <DropdownMenuItem onClick={onInsertExcalidraw}>
          <Pencil />
          Excalidraw drawing
        </DropdownMenuItem>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <Code2 />
            Code block
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            {CODE_LANGUAGES.map((lang) => (
              <DropdownMenuItem key={lang} onClick={() => onInsertCodeBlock(lang)}>
                {lang}
              </DropdownMenuItem>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuItem onClick={onInsertMermaid}>
          <ChartNetwork />
          Mermaid diagram
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function MarkdownChatToolbar({ editorView, onInsertWidget, onSubmit, isStreaming, onStop }: ToolbarProps) {
  const v = editorView

  return (
    // The Send/Stop primary action lives OUTSIDE the base-ui <Toolbar>: that
    // component manages its children as a roving-focus composite, and a plain
    // <Button> placed inside it as a non-registered child doesn't receive click
    // activation (only the ToolbarButton composite items do). Keeping it as a
    // sibling guarantees its onClick fires.
    <div
      className="flex items-center py-1.5"
      style={{ paddingLeft: 'max(48px, calc((100% - 680px) / 2 + 48px))', paddingRight: 'max(48px, calc((100% - 680px) / 2 + 48px))' }}
    >
      <Toolbar className="rounded-none border-0 bg-transparent p-0 shadow-none">
        <ToolbarGroup>
          <ToolbarButton
            aria-label="Bold"
            onClick={() => v && wrapSelection(v, '**')}
            className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-xs font-bold text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            B
          </ToolbarButton>
          <ToolbarButton
            aria-label="Italic"
            onClick={() => v && wrapSelection(v, '*')}
            className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-xs italic text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            I
          </ToolbarButton>
          <ToolbarButton
            aria-label="Inline code"
            onClick={() => v && wrapSelection(v, '`')}
            className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 font-mono text-[10px] text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            {"`x`"}
          </ToolbarButton>
        </ToolbarGroup>

        <ToolbarSeparator className="bg-border" />

        <ToolbarGroup>
          <ToolbarButton
            aria-label="Heading 1"
            onClick={() => v && prependLine(v, '# ')}
            className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-xs font-semibold text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            H1
          </ToolbarButton>
          <ToolbarButton
            aria-label="Heading 2"
            onClick={() => v && prependLine(v, '## ')}
            className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-xs font-semibold text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            H2
          </ToolbarButton>
          <ToolbarButton
            aria-label="Heading 3"
            onClick={() => v && prependLine(v, '### ')}
            className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-xs font-semibold text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            H3
          </ToolbarButton>
        </ToolbarGroup>

        <ToolbarSeparator className="bg-border" />

        <InsertDropdown
          onInsertExcalidraw={() => {
            if (!v) return
            const id = nanoid()
            // appendWidget BEFORE inserting into CM6 so FencedWidget.toDOM() finds it
            onInsertWidget('excalidraw', id)
            insertExcalidraw(v, id)
          }}
          onInsertCodeBlock={(lang) => v && insertCodeBlock(v, lang)}
          onInsertMermaid={() => v && insertMermaid(v)}
        />
      </Toolbar>

      <div className="ml-auto flex items-center gap-2">
        {isStreaming ? (
          <Button
            variant="destructive"
            size="icon-sm"
            onClick={onStop}
            title="Stop"
          >
            <Square />
          </Button>
        ) : (
          <Button
            variant="default"
            size="icon-sm"
            onClick={onSubmit}
            title="Send (⌘↵)"
          >
            <SendHorizontal />
          </Button>
        )}
      </div>
    </div>
  )
}
