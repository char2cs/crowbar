import { useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { HostStatusIcon } from '@/features/hosts/components/host-status-icon'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import { hostDotLabel } from '@/features/hosts/lib/host-tone'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { dataOf } from '@/lib/loadable'
import { EMPTY_PROJECTS, useProjectDataStore, useProjectStore } from '@/lib/store/projects'

interface HostProjectsPendingProps {
  onProject: (projectId: string) => void
  onAddHost: () => void
}

/** The phone app's landing while its paired hosts have listed no project: each host's status,
 *  and the active (or first) project opened the moment one lands. */
export function HostProjectsPending({ onProject, onAddHost }: HostProjectsPendingProps) {
  const hosts = useHostConfigStore((s) => s.hosts)
  const projects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)

  useEffect(() => {
    if (projects.length === 0) return
    const activeId = useProjectStore.getState().activeProjectId
    onProject((projects.find((p) => p.id === activeId) ?? projects[0]).id)
  }, [projects, onProject])

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-4 px-6 text-center">
      <p className="text-sm text-muted-foreground">
        Waiting for your hosts to list their projects.
      </p>
      <ul className="flex w-full max-w-xs flex-col gap-2">
        {hosts.map((host) => (
          <HostLine key={host.id} hostId={host.id} />
        ))}
      </ul>
      <Button variant="outline" onClick={onAddHost}>
        Pair another host
      </Button>
    </div>
  )
}

function HostLine({ hostId }: { hostId: string }) {
  const { name, state } = useHostView(hostId)
  return (
    <li className="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm">
      <HostStatusIcon hostId={hostId} state={state} />
      <span className="min-w-0 flex-1 truncate text-left">{name}</span>
      <span className="text-[12px] text-muted-foreground">{hostDotLabel(state)}</span>
    </li>
  )
}
