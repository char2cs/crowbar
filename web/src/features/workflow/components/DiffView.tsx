import { FileText } from 'lucide-react'

interface DiffViewProps {
  workspaceId: string
  stepId: string
}

const STEP_LABELS: Record<string, { title: string; description: string }> = {
  spec: {
    title: 'Specification',
    description: 'The spec will appear here once generated. Start by describing the feature in the Brainstorm step.',
  },
  ai_review: {
    title: 'AI Review',
    description: 'AI-generated review comments will appear here after the build step is complete.',
  },
}

export function DiffView({ stepId }: DiffViewProps) {
  const label = STEP_LABELS[stepId] ?? {
    title: 'Diff',
    description: 'Changes will appear here.',
  }

  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
      <FileText className="size-8 opacity-30" />
      <div className="text-center">
        <p className="text-sm font-medium text-text">{label.title}</p>
        <p className="mt-1 max-w-xs text-xs text-text-lighter">{label.description}</p>
      </div>
    </div>
  )
}
