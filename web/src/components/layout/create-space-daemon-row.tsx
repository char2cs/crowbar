import { useState } from 'react'
import { HardDrives } from '@phosphor-icons/react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectPrimitive,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'
import {
  HostStatusDot,
  hostDotLabel,
  hostIsUnreachable,
} from '@/features/hosts/components/host-status-dot'
import { useHostDotState, useHostView } from '@/features/hosts/hooks/use-host-status'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST, type Host } from '@/features/hosts/types'

// A trailing pick that opens AddHostDialog rather than ever becoming the selected daemon.
const ADD_HOST_VALUE = '__add-host__'

/** One selectable saved host, disabled (not hidden) while nothing can be asked of it — a
 *  project can't be created on a host that can't currently be reached. */
function DaemonOption({ host }: { host: Host }) {
  const state = useHostDotState(host.id)
  return (
    <SelectItem value={host.id} disabled={hostIsUnreachable(state)}>
      <span className="flex items-center gap-2">
        <HostStatusDot hostId={host.id} state={state} />
        {host.name}
      </span>
    </SelectItem>
  )
}

function LocalOption() {
  const state = useHostDotState(LOCAL_HOST.id)
  return (
    <SelectItem value={LOCAL_HOST.id}>
      <span className="flex items-center gap-2">
        <HostStatusDot hostId={LOCAL_HOST.id} state={state} />
        {LOCAL_HOST.name}
      </span>
    </SelectItem>
  )
}

interface CreateSpaceDaemonRowProps {
  hostId: string
  onHostIdChange: (hostId: string) => void
}

/** Create Space's Daemon row: Local, every saved host with its status, and "+ Add a host…". */
export function CreateSpaceDaemonRow({ hostId, onHostIdChange }: CreateSpaceDaemonRowProps) {
  const hosts = useHostConfigStore((s) => s.hosts)
  const selected = useHostView(hostId)
  const [addHostOpen, setAddHostOpen] = useState(false)
  // Each opening is a new attempt, even one made while the last is still closing.
  const [addHostAttempt, setAddHostAttempt] = useState(0)

  function handleValueChange(value: string | null) {
    if (!value) return
    if (value === ADD_HOST_VALUE) {
      setAddHostAttempt((n) => n + 1)
      setAddHostOpen(true)
      return
    }
    onHostIdChange(value)
  }

  return (
    <div className="flex items-center justify-between rounded-lg border border-border bg-muted px-3 py-2.5 text-sm">
      <span className="flex items-center gap-2 text-foreground">
        <HardDrives size={16} className="text-muted-foreground" />
        Daemon
      </span>
      <Select value={hostId} onValueChange={handleValueChange}>
        <SelectTrigger
          aria-label="Daemon"
          size="sm"
          className="h-auto min-h-0 w-auto min-w-0 border-none bg-transparent px-0 text-muted-foreground shadow-none before:hidden hover:bg-transparent"
        >
          <SelectValue>
            <span className="flex items-center gap-1.5">
              <HostStatusDot hostId={hostId} state={selected.state} />
              {selected.name ?? hostDotLabel(selected.state)}
            </span>
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <LocalOption />
          {hosts.length > 0 && <SelectPrimitive.Separator className="my-1 h-px bg-border" />}
          {hosts.map((host) => (
            <DaemonOption key={host.id} host={host} />
          ))}
          <SelectPrimitive.Separator className="my-1 h-px bg-border" />
          <SelectItem value={ADD_HOST_VALUE}>+ Add a host…</SelectItem>
        </SelectContent>
      </Select>
      <AddHostDialog key={addHostAttempt} open={addHostOpen} onOpenChange={setAddHostOpen} />
    </div>
  )
}
