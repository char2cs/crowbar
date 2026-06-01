import CodeMirror from '@uiw/react-codemirror'
import { markdown } from '@codemirror/lang-markdown'
import { useSidebarStore } from '@/lib/store/sidebar'
import { Frame, FramePanel, FrameHeader, FrameTitle } from '@/components/ui/frame'
import { cn } from '@/utils/cn'

interface AboutTabProps {
  description: string
  onDescriptionChange: (value: string) => void
  onOpenConversation: (id: string) => void
}

export function AboutTab({ description, onDescriptionChange, onOpenConversation }: AboutTabProps) {
  const chats = useSidebarStore(s => s.chats)

  return (
    <div className="flex flex-col gap-3 p-4">
      <Frame>
        <FramePanel className="p-0 overflow-hidden">
          <FrameHeader className="px-4 pt-3 pb-2">
            <FrameTitle>Description</FrameTitle>
          </FrameHeader>
          <CodeMirror
            value={description}
            extensions={[markdown()]}
            onChange={onDescriptionChange}
            basicSetup={{ lineNumbers: false, foldGutter: false, dropCursor: false, allowMultipleSelections: false, indentOnInput: true }}
            className="text-xs"
          />
        </FramePanel>
      </Frame>

      <Frame>
        <FramePanel>
          <FrameHeader className="px-0 pt-0 pb-3">
            <FrameTitle>Conversations</FrameTitle>
          </FrameHeader>
          {chats.length === 0 ? (
            <p className="text-xs text-muted-foreground/40">No conversations yet.</p>
          ) : (
            <div className="flex flex-col gap-1">
              {chats.map(chat => (
                <button
                  key={chat.id}
                  onClick={() => onOpenConversation(chat.id)}
                  className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left transition-colors hover:bg-accent"
                >
                  <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full',
                    chat.age === 'active' ? 'bg-green-500' : 'bg-muted-foreground/30')} />
                  <span className="flex-1 truncate text-xs text-foreground">{chat.title}</span>
                  <span className="text-xs text-muted-foreground/50">{chat.age}</span>
                </button>
              ))}
            </div>
          )}
        </FramePanel>
      </Frame>
    </div>
  )
}
