import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { workspaceQueryOptions } from '@/lib/queries'
import { MarkdownChatView } from '@/features/markdown-chat/components/markdown-chat-view'
import { DiffView } from '@/components/review/DiffView'

export const Route = createFileRoute('/workspaces/$wsId/$step')({
  component: StepPage,
})

function StepPage() {
  const { wsId, step } = Route.useParams()
  const { data: workspace, isError, isPending } = useQuery(workspaceQueryOptions(wsId))

  if (isPending) return null

  if (isError || !workspace) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
        <p>Workspace not found.</p>
        <Link to="/" className="underline hover:text-foreground">Back to home</Link>
      </div>
    )
  }

  const stateDef = workspace.flow.states.find(s => s.name === step)
  const ui = stateDef?.ui ?? 'chat'

  if (ui === 'diff') return <DiffView workspaceId={wsId} step={step} />
  return <MarkdownChatView workspaceId={wsId} stepId={step} />
}
