interface ChatViewProps {
  workspaceId: string
  stepId: string
}

export function ChatView({ workspaceId, stepId }: ChatViewProps) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
      <p className="text-sm">Chat — workspace: {workspaceId}, step: {stepId}</p>
    </div>
  )
}
