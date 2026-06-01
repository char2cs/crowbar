import CodeMirror from '@uiw/react-codemirror'
import { markdown } from '@codemirror/lang-markdown'
import { useSidebarStore } from '@/lib/store/sidebar'
import { cn } from '@/utils/cn'

interface AboutTabProps {
  description: string
  onDescriptionChange: (value: string) => void
  onOpenConversation: (id: string) => void
}

export function AboutTab({ description, onDescriptionChange, onOpenConversation }: AboutTabProps) {
  const chats = useSidebarStore(s => s.chats)

  return (
    <div className="flex flex-col divide-y divide-border">
      <div className="p-4">
        <h3 className="mb-2.5 text-xs font-semibold text-foreground">Description</h3>
        <div className="overflow-hidden rounded-lg border border-border bg-[#0d0d0d]">
          <CodeMirror
            value={description}
            extensions={[markdown()]}
            onChange={onDescriptionChange}
            basicSetup={{ lineNumbers: false, foldGutter: false, dropCursor: false, allowMultipleSelections: false, indentOnInput: true }}
            className="text-xs"
          />
        </div>
      </div>

      <div className="p-4">
        <h3 className="mb-2.5 text-xs font-semibold text-foreground">Conversations</h3>
        {chats.length === 0 ? (
          <p className="text-xs text-muted-foreground/40">No conversations yet.</p>
        ) : (
          <div className="flex flex-col gap-0.5">
            {chats.map(chat => (
              <button
                key={chat.id}
                onClick={() => onOpenConversation(chat.id)}
                className="flex w-full items-center gap-2.5 rounded-lg border border-border bg-[#0d0d0d] px-3 py-2 text-left transition-colors hover:bg-muted/40"
              >
                <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full',
                  chat.age === 'active' ? 'bg-green-500' : 'bg-muted-foreground/30')} />
                <span className="flex-1 truncate text-xs text-foreground">{chat.title}</span>
                <span className="text-xs text-muted-foreground/50">{chat.age}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
