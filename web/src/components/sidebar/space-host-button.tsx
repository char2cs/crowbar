import { ROW_SUB_ACTION } from '@/components/layout/workspace-row-base'
import { WithTooltip } from '@/components/ui/tooltip'
import { HostStatusIcon } from '@/features/hosts/components/host-status-icon'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import { hostDotLabel, hostTone, hostToneText } from '@/features/hosts/lib/host-tone'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { cn } from '@/lib/utils'

const BADGE = cn(ROW_SUB_ACTION, 'items-center gap-1 text-xs whitespace-nowrap')

/** A remote project's host at a glance — its computer and status dot — and the way into
 *  Settings → Hosts, where that host is repaired or revoked. */
export function SpaceHostButton({ hostId }: { hostId: string }) {
  const { name, state } = useHostView(hostId)
  const stateWord = hostDotLabel(state)
  const label = `Host settings: ${name === undefined ? '' : `${name}, `}${stateWord}`
  // Quiet while the host is online; short of that it takes the state's colour and says it.
  const online = hostTone(state) === 'online'
  const tone = online ? { className: BADGE } : hostToneText(state, BADGE)

  return (
    <WithTooltip
      content={label}
      trigger={
        <button
          type="button"
          data-control="host-settings"
          {...tone}
          aria-label={label}
          onClick={() => useUIState.getState().openSettingsDialog('hosts')}
        >
          <HostStatusIcon
            hostId={hostId}
            state={state}
            weight="bold"
            showTitle={false}
            className="size-3.5"
          />
          {!online && <span>{stateWord}</span>}
        </button>
      }
    />
  )
}
