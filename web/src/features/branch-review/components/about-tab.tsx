import { useState, useEffect, useCallback } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Plus } from '@phosphor-icons/react'
import CodeMirror from '@uiw/react-codemirror'
import { markdown } from '@codemirror/lang-markdown'
import { FramePanel, FrameTitle } from '@/components/ui/frame'
import { DataState } from '@/components/ui/data-state'
import { Button } from '@/components/ui/button'
import { useBranchReviewChatsStore } from '@/features/branch-review/stores/branch-review-data-store'
import { transparentMarkdownTheme } from '../lib/markdown'
import { cn } from '@/utils/cn'

interface AboutTabProps {
  wsId: string
  description: string
  onDescriptionChange: (value: string) => void
  onOpenConversation: (id: string) => void
  onNewConversation: () => void
}

export function AboutTab({ wsId, description, onDescriptionChange, onOpenConversation, onNewConversation }: AboutTabProps) {
  const [editing, setEditing] = useState(false)
  const chatsLoadable = useBranchReviewChatsStore(s => s.data)
  const retryChats = useCallback(() => { void useBranchReviewChatsStore.getState().fetch(wsId) }, [wsId])
  useEffect(() => { void useBranchReviewChatsStore.getState().fetch(wsId) }, [wsId])

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <FrameTitle className="text-base">Description</FrameTitle>
        {editing ? (
          <CodeMirror
            autoFocus
            value={description}
            placeholder="Describe what this branch does, its goals, and any context needed for review…"
            extensions={[markdown(), transparentMarkdownTheme]}
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

      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <FrameTitle className="text-base">Conversations</FrameTitle>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onNewConversation}
            tooltip="New conversation"
            aria-label="New conversation"
          >
            <Plus weight="bold" size={12} />
          </Button>
        </div>
        <DataState
          loadable={chatsLoadable}
          onRetry={retryChats}
          loadingLabel="Loading conversations"
          emptyMessage="No conversations yet."
          isEmpty={(d) => d.length === 0}
        >
          {(chats) => (
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
                    <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full', chat.isActive ? 'bg-success' : 'bg-muted-foreground/30')} />
                    <span className="flex-1 truncate text-sm text-foreground">{chat.title}</span>
                    <span className="text-xs text-muted-foreground/50">{chat.age}</span>
                  </div>
                </FramePanel>
              ))}
            </div>
          )}
        </DataState>
      </div>
    </div>
  )
}
