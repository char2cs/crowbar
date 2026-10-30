import { Plus, Trash } from '@phosphor-icons/react'
import { useCallback, useRef, useState, type ReactNode, type Ref } from 'react'
import { Button } from '@/components/ui/button'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'
import { HostStatusIcon } from '@/features/hosts/components/host-status-icon'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import { revokeHost } from '@/features/hosts/lib/host-revoke'
import { hostDotLabel, hostToneText } from '@/features/hosts/lib/host-tone'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST, type Host } from '@/features/hosts/types'

// Unless another modal now covers Settings: focus the user took there is theirs to keep.
function focusUnlessCovered(target: HTMLElement | null) {
  if (target && !target.closest('[aria-hidden="true"], [inert]')) target.focus()
}

/** What a host row draws. */
type HostSummary = Pick<Host, 'id' | 'name' | 'endpoint'>

interface HostRowProps<H extends HostSummary> {
  host: H
  isLocal: boolean
  revoking: boolean
  onRevoke: (host: H) => void
}

const SECONDARY = 'ui-font ui-text-sm truncate text-muted-foreground'

function HostRow<H extends HostSummary>({ host, isLocal, revoking, onRevoke }: HostRowProps<H>) {
  const { state } = useHostView(host.id)
  const summary = (
    <>
      <HostStatusIcon hostId={host.id} state={state} className="text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <div className="ui-font ui-text-sm truncate text-foreground">{host.name}</div>
        {/* Local is the app itself: its line is never tinted. */}
        <div {...(isLocal ? { className: SECONDARY } : hostToneText(state, SECONDARY))}>
          {isLocal ? 'This computer' : host.endpoint}
          {state !== 'up' && ` · ${hostDotLabel(state)}`}
        </div>
      </div>
    </>
  )

  return (
    <div
      data-testid="host-row"
      className="flex items-center gap-3 rounded-lg px-1 py-2 transition-colors hover:bg-muted/50"
    >
      {summary}
      {!isLocal && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          // Not `disabled`: the confirm returns focus here, and a disabled button takes none.
          aria-disabled={revoking || undefined}
          aria-label={`Revoke ${host.name}`}
          tooltip={`Revoke ${host.name}`}
          className="text-muted-foreground hover:text-destructive aria-disabled:pointer-events-none aria-disabled:opacity-64"
          onClick={() => onRevoke(host)}
        >
          <Trash />
        </Button>
      )}
    </div>
  )
}

interface HostsSettingsViewProps<H extends HostSummary> {
  hosts: readonly H[]
  description: string
  revoking: ReadonlySet<string>
  onRevoke: (host: H) => void
  onAdd: () => void
  addHostRef?: Ref<HTMLButtonElement>
  children?: ReactNode
}

/** Settings → Hosts as drawn: every host with its status, revoke, and Add host. */
function HostsSettingsView<H extends HostSummary>({
  hosts,
  description,
  revoking,
  onRevoke,
  onAdd,
  addHostRef,
  children,
}: HostsSettingsViewProps<H>) {
  return (
    <div className="space-y-4">
      <div className="mb-2 px-1 py-1.5">
        <h3 className="font-heading text-[32px] text-foreground">Hosts</h3>
        <p className="ui-font ui-text-sm text-muted-foreground">{description}</p>
      </div>

      <div className="space-y-1">
        <HostRow host={LOCAL_HOST} isLocal revoking={false} onRevoke={() => {}} />
        {hosts.map((host) => (
          <HostRow
            key={host.id}
            host={host}
            isLocal={false}
            revoking={revoking.has(host.id)}
            onRevoke={onRevoke}
          />
        ))}
      </div>

      <div className="px-1">
        <Button ref={addHostRef} type="button" variant="outline" size="sm" onClick={onAdd}>
          <Plus />
          Add host
        </Button>
      </div>
      {children}
    </div>
  )
}

/** Settings → Hosts: Local and every paired daemon with its status, Add host, and revoke.
 *  Desktop-only: SETTINGS_TAB_ITEMS leaves it out of a non-Tauri build. */
export function HostsSettings() {
  const hosts = useHostConfigStore((s) => s.hosts)
  const [addOpen, setAddOpen] = useState(false)
  // Each opening is a new attempt, even one made while the last is still closing.
  const [addAttempt, setAddAttempt] = useState(0)
  const addHostRef = useRef<HTMLButtonElement>(null)
  // Every host whose revoke is under way: none of them can be asked to revoke twice.
  const [revoking, setRevoking] = useState<ReadonlySet<string>>(() => new Set())

  const handleRevoke = useCallback(
    (host: Host) => {
      if (revoking.has(host.id)) return
      setRevoking((ids) => new Set(ids).add(host.id))
      void revokeHost(host)
        .then((removed) => {
          // The row that held the user's place is gone: Add host is the one control that stays.
          if (removed) focusUnlessCovered(addHostRef.current)
        })
        .finally(() => {
          setRevoking((ids) => {
            const next = new Set(ids)
            next.delete(host.id)
            return next
          })
        })
    },
    [revoking],
  )

  return (
    <HostsSettingsView
      hosts={hosts}
      description="Crowbar always runs on this computer. Add another machine running its own daemon to reach it from here too."
      revoking={revoking}
      onRevoke={handleRevoke}
      onAdd={() => {
        setAddAttempt((n) => n + 1)
        setAddOpen(true)
      }}
      addHostRef={addHostRef}
    >
      <AddHostDialog key={addAttempt} open={addOpen} onOpenChange={setAddOpen} />
    </HostsSettingsView>
  )
}
