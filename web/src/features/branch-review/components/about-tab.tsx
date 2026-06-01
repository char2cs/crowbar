import CodeMirror from '@uiw/react-codemirror'
import { EditorView } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { useSidebarStore } from '@/lib/store/sidebar'
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
  description: string
  onDescriptionChange: (value: string) => void
  onOpenConversation: (id: string) => void
}

export function AboutTab({ description, onDescriptionChange, onOpenConversation }: AboutTabProps) {
  const chats = useSidebarStore(s => s.chats)

  return (
    <div className="flex flex-col gap-4">
      {/* Description */}
      <div className="flex flex-col gap-2">
        <FrameTitle>Description</FrameTitle>
        <CodeMirror
          value={description}
          placeholder="Describe what this branch does, its goals, and any context needed for review…"
          extensions={[markdown(), transparentTheme]}
          onChange={onDescriptionChange}
          basicSetup={{ lineNumbers: false, foldGutter: false, dropCursor: false, allowMultipleSelections: false, indentOnInput: true }}
          className="text-sm"
        />
      </div>

      {/* Conversations */}
      <div className="flex flex-col gap-2">
        <FrameTitle>Conversations</FrameTitle>
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
                    chat.age === 'active' ? 'bg-green-500' : 'bg-muted-foreground/30')} />
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
