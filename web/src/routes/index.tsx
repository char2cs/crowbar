import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '../lib/transport'
import type { HealthStatus } from '../domain/health'

export const Route = createFileRoute('/')({
  component: IndexPage,
})

function IndexPage() {
  const { data, isLoading, error } = useQuery<HealthStatus>({
    queryKey: ['health'],
    queryFn: () => apiFetch('/api/v0/health').then((r) => r.json()),
  })

  if (isLoading) return <p className="p-8 text-zinc-400">Connecting to daemon...</p>
  if (error) return <p className="p-8 text-red-400">Daemon unreachable</p>

  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold mb-2">Crowbar</h1>
      <p className="text-zinc-400">
        Status: <span className="text-green-400">{data?.status}</span>
      </p>
      <p className="text-zinc-400">Version: {data?.version}</p>
    </div>
  )
}
