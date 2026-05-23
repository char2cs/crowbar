export function ChatEmptyState() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 py-24 text-center">
      <span className="text-2xl text-primary/50">&#10022;</span>
      <p className="text-sm font-medium text-foreground">Start a conversation</p>
      <p className="text-xs text-muted-foreground">Ask anything about this workspace</p>
    </div>
  )
}
