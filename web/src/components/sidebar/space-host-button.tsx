import { ROW_SUB_ACTION } from '@/components/layout/workspace-row-base'
import { WithTooltip } from '@/components/ui/tooltip'
import { HostStatusIcon } from '@/features/hosts/components/host-status-icon'
import { useProjectHost } from '@/features/hosts/hooks/use-host-status'
import { hostDotLabel, hostToneText } from '@/features/hosts/lib/host-tone'
import { useUIState } from '@/features/window/stores/ui-state-store'

/** A remote project's host at a glance — its computer and status dot — and the way into
 *  Settings → Hosts, where that host is repaired or revoked. */
export function SpaceHostButton({ hostId }: { hostId: string }) {
  const { name, state, unreachable } = useProjectHost(hostId)
  const label = `Host settings: ${name === undefined ? '' : `${name}, `}${hostDotLabel(state)}`
  // At rest the computer is as quiet as the row's other actions; a host that can't be reached turns it.
  const tone = unreachable ? hostToneText(state, ROW_SUB_ACTION) : { className: ROW_SUB_ACTION }

  return (
    <WithTooltip
      content={label}
      trigger={
        <button
          type="button"
          data-control="host-settings"
          {...tone}
          aria-label={label}
          onClick={(e) => {
            e.stopPropagation()
            useUIState.getState().openSettingsDialog('hosts')
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <HostStatusIcon
            hostId={hostId}
            state={state}
            weight="bold"
            showTitle={false}
            className="size-3.5"
          />
        </button>
      }
    />
  )
}
