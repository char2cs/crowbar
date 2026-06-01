import { useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import CodeMirror from '@uiw/react-codemirror'
import { EditorView } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { getMockBranchReviewChats } from '@/lib/mock/branch-diff'
import { FramePanel, FrameTitle } from '@/components/ui/frame'
import { cn } from '@/utils/cn'

const transparentTheme = EditorView.theme({
  '&': { backgroundColor: 'transparent !important', color: 'var(--foreground)' },
  '&.cm-focused': { outline: 'none !important', backgroundColor: 'transparent !important' },
  '.cm-content': { caretColor: 'var(--foreground)', padding: '0' },
  '.cm-cursor': { borderLeftColor: 'var(--foreground)' },
  '.cm-placeholder': { color: 'var(--muted-foreground)', opacity: '0.4' },
  '.cm-line': { padding: '0' },
  '.cm-scroller': { fontFamily: 'inherit', backgroundColor: 'transparent !important' },
  '.cm-gutters': { backgroundColor: 'transparent !important', border: 'none' },
  '.cm-activeLine': { backgroundColor: 'transparent !important' },
  '.cm-activeLineGutter': { backgroundColor: 'transparent !important' },
  '.cm-selectionBackground': { backgroundColor: 'color-mix(in srgb, var(--primary) 20%, transparent) !important' },
  '&.cm-focused .cm-selectionBackground': { backgroundColor: 'color-mix(in srgb, var(--primary) 30%, transparent) !important' },
})

interface AboutTabProps {
  wsId: string
  description: string
  onDescriptionChange: (value: string) => void
  onOpenConversation: (id: string) => void
}

export function AboutTab({ wsId, description, onDescriptionChange, onOpenConversation }: AboutTabProps) {
  const [editing, setEditing] = useState(false)
  const chats = getMockBranchReviewChats(wsId)

  return (
    <div className="flex flex-col gap-4">
      {/* Description — rendered markdown, click to edit */}
      <div className="flex flex-col gap-2">
        <FrameTitle className="text-base">Description</FrameTitle>
        {editing ? (
          <CodeMirror
            autoFocus
            value={description}
            placeholder="Describe what this branch does, its goals, and any context needed for review…"
            extensions={[markdown(), transparentTheme]}
            onChange={onDescriptionChange}
            onBlur={() => setEditing(false)}
            basicSetup={{ lineNumbers: false, foldGutter: false, dropCursor: false, allowMultipleSelections: false, indentOnInput: true }}
            className="text-sm"
          />
        ) : description ? (
          <div
            onClick={() => setEditing(true)}
            className="prose prose-sm prose-invert max-w-none cursor-text text-sm text-foreground
              [&_h1]:text-base [&_h1]:font-semibold [&_h1]:mb-2 [&_h1]:mt-3
              [&_h2]:text-sm [&_h2]:font-semibold [&_h2]:mb-1.5 [&_h2]:mt-3
              [&_h3]:text-sm [&_h3]:font-medium [&_h3]:mb-1 [&_h3]:mt-2
              [&_p]:mb-2 [&_p]:leading-relaxed
              [&_ul]:my-1.5 [&_ul]:pl-4 [&_li]:my-0.5
              [&_ol]:my-1.5 [&_ol]:pl-4
              [&_code]:rounded [&_code]:bg-muted/60 [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-xs [&_code]:font-mono
              [&_pre]:rounded-lg [&_pre]:bg-muted/60 [&_pre]:p-3 [&_pre]:text-xs [&_pre]:overflow-x-auto
              [&_pre_code]:bg-transparent [&_pre_code]:p-0
              [&_strong]:font-semibold [&_strong]:text-foreground
              [&_em]:italic
              [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground
              [&_hr]:border-border [&_hr]:my-3
              [&_a]:text-primary [&_a]:underline-offset-2 [&_a]:hover:underline"
          >
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{description}</ReactMarkdown>
          </div>
        ) : (
          <p
            onClick={() => setEditing(true)}
            className="cursor-text text-sm text-muted-foreground/40"
          >
            Describe what this branch does, its goals, and any context needed for review…
          </p>
        )}
      </div>

      {/* Conversations */}
      <div className="flex flex-col gap-2">
        <FrameTitle className="text-base">Conversations</FrameTitle>
        {chats.length === 0 ? (
          <p className="text-sm text-muted-foreground/40">No conversations yet.</p>
        ) : (
          <div className="flex flex-col gap-1.5">
            {chats.map(chat => (
              <FramePanel
                key={chat.id}
                className="cursor-pointer py-2.5 px-3 transition-colors hover:bg-accent/20"
                onClick={() => onOpenConversation(chat.id)}
                role="button"
                tabIndex={0}
              >
                <div className="flex items-center gap-2.5">
                  <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full',
                    chat.isActive ? 'bg-green-500' : 'bg-muted-foreground/30')} />
                  <span className="flex-1 truncate text-sm text-foreground">{chat.title}</span>
                  <span className="text-xs text-muted-foreground/50">{chat.age}</span>
                </div>
              </FramePanel>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
