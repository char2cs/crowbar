// Temporary stub — replaced in Task 24
export function FlowContent({ workspaceId }: { workspaceId: string }) {
  return (
    <div className="flex h-full items-center justify-center text-muted-foreground">
      <p className="text-sm">Loading workspace {workspaceId}…</p>
    </div>
  )
}
