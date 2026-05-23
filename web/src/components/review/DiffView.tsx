interface DiffViewProps {
  workspaceId: string
  step: string
}

export function DiffView({ workspaceId, step }: DiffViewProps) {
  return (
    <div className="flex flex-1 items-center justify-center text-muted-foreground">
      <div className="text-center">
        <div className="text-sm font-medium">Review view coming soon</div>
        <div className="mt-1 text-xs">{workspaceId} · {step}</div>
      </div>
    </div>
  )
}
