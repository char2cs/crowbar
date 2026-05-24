interface DiffViewProps {
  workspaceId: string
  stepId: string
}

export function DiffView({ workspaceId, stepId }: DiffViewProps) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
      <p className="text-sm">Diff — workspace: {workspaceId}, step: {stepId}</p>
    </div>
  )
}
