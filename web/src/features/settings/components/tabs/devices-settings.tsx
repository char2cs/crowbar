import { DeviceMobile, Plus, Trash } from '@phosphor-icons/react'
import { useState } from 'react'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { SettingSelect } from '@/features/settings/components/setting-select'
import { SETTINGS_CONTROL_WIDTHS } from '@/features/settings/components/settings-control-widths'
import Section, { SettingRow } from '@/features/settings/components/settings-section'
import { AddDeviceDialog, type Minted } from '@/features/hosts/components/add-device-dialog'
import { useDeviceAccess } from '@/features/hosts/hooks/use-device-access'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import type { PairedDevice } from '@/features/hosts/lib/device-access'
import { sidebarHostIds } from '@/features/hosts/lib/host-filter'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'

const SECONDARY = 'ui-font ui-text-sm truncate text-muted-foreground'

const day = (iso: string) =>
  new Date(iso).toLocaleDateString([], { year: 'numeric', month: 'short', day: 'numeric' })

function deviceDetail(device: PairedDevice, isThisComputer: boolean) {
  if (isThisComputer) return 'This computer'
  const paired = `Paired ${day(device.pairedAt)}`
  return device.lastSeen ? `${paired} · Last seen ${day(device.lastSeen)}` : paired
}

interface DeviceRowProps {
  device: PairedDevice
  isThisComputer: boolean
  onRevoke: (device: PairedDevice) => void
}

function DeviceRow({ device, isThisComputer, onRevoke }: DeviceRowProps) {
  return (
    <div
      data-testid="device-row"
      className="flex items-center gap-3 rounded-lg px-1 py-2 transition-colors hover:bg-muted/50"
    >
      <DeviceMobile className="size-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <div className="ui-font ui-text-sm truncate text-foreground">{device.name}</div>
        <div className={SECONDARY}>{deviceDetail(device, isThisComputer)}</div>
      </div>
      {/* This computer's own key is how it reaches this instance: removing a host revokes it. */}
      {!isThisComputer && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={`Revoke ${device.name}`}
          tooltip={`Revoke ${device.name}`}
          className="text-muted-foreground hover:text-destructive"
          onClick={() => onRevoke(device)}
        >
          <Trash />
        </Button>
      )}
    </div>
  )
}

/** Settings → Devices: who may reach the connected instance, local or remote, from another device. */
export function DevicesSettings() {
  const paired = useHostConfigStore((s) => s.hosts)
  const hostIds = sidebarHostIds(paired)
  const [chosen, setChosen] = useState<string | null>(null)
  // Never the sidebar's filter: a stale offline host there would open this tab on an error.
  const hostId = chosen && hostIds.includes(chosen) ? chosen : LOCAL_HOST.id
  const { name } = useHostView(hostId)
  // A remote reaches the daemon over TLS, which may read these settings but not change them.
  const remote = hostId !== LOCAL_HOST.id
  const thisComputerId = paired.find((h) => h.id === hostId)?.clientId
  const hostOptions = hostIds.map((id) => ({
    value: id,
    label: id === LOCAL_HOST.id ? LOCAL_HOST.name : (paired.find((h) => h.id === id)?.name ?? id),
  }))

  const view = useDeviceAccess(hostId)
  const { access } = view
  const [draft, setDraft] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [minted, setMinted] = useState<Minted | null>(null)

  const address = draft ?? access?.address ?? ''

  async function toggle(enabled: boolean) {
    setBusy(true)
    setError(null)
    try {
      const failure = await view.setEnabled(enabled, address.trim())
      setError(failure)
      // A refused address stays as typed so it can be corrected rather than retyped.
      if (!failure) setDraft(null)
    } finally {
      setBusy(false)
    }
  }

  function newCode() {
    setMinted({ state: 'minting' })
    // A late answer lands only if the popup is still waiting for it.
    const settle = (next: Minted) => setMinted((cur) => (cur?.state === 'minting' ? next : cur))
    view.addDevice().then(
      (offer) => settle({ state: 'ready', offer }),
      () => settle({ state: 'failed' }),
    )
  }

  async function closeOffer() {
    setMinted(null)
    setError(await view.refreshDevices())
  }

  async function revoke(device: PairedDevice) {
    const confirmed = await primitiveConfirm(
      `${device.name} will be disconnected and will need a new code to connect again.`,
      {
        title: `Revoke ${device.name}?`,
        confirmLabel: 'Revoke',
        cancelLabel: 'Keep it',
        destructive: true,
      },
    )
    if (confirmed) setError(await view.revoke(device.id))
  }

  return (
    <div className="space-y-4">
      <div className="mb-2 px-1 py-1.5">
        <h3 className="font-heading text-[32px] text-foreground">Devices</h3>
        <p className="ui-font ui-text-sm text-muted-foreground">
          Let a phone or another computer connect to {name ?? 'this instance'}. Each device pairs
          once with a code and can be revoked here.
        </p>
      </div>

      {/* The wrapper makes this the first section of its parent, so its own title stays hidden
          under the tab's. */}
      <div>
        <Section title="Device connections">
          {hostIds.length > 1 && (
            <SettingRow label="Instance" description="Which Crowbar these settings apply to">
              <SettingSelect
                options={hostOptions}
                value={hostId}
                width="wide"
                onValueChange={(next) => next && setChosen(next)}
              />
            </SettingRow>
          )}

          {view.load.status === 'failed' && (
            <div
              role="alert"
              className="ui-font ui-text-sm flex items-center gap-2 px-1 py-2 text-destructive"
            >
              <span className="min-w-0 flex-1">{view.load.why}</span>
              <Button type="button" variant="outline" size="sm" onClick={view.retry}>
                Try again
              </Button>
            </div>
          )}

          {view.load.status === 'ready' && access && (
            <>
              {remote ? (
                <SettingRow
                  label="Device connections"
                  description={`This can only be changed on the computer ${name ?? 'this instance'} runs on.`}
                >
                  <span className="ui-font ui-text-sm text-muted-foreground">
                    {access.enabled ? 'On' : 'Off'}
                  </span>
                </SettingRow>
              ) : (
                <SettingRow
                  label="Allow device connections"
                  description={
                    access.enabled
                      ? 'Devices with a code can connect while this is on.'
                      : 'Off. Nothing listens until you turn this on.'
                  }
                >
                  <Switch
                    aria-label="Allow device connections"
                    size="sm"
                    checked={access.enabled}
                    disabled={busy}
                    onCheckedChange={(next) => void toggle(next)}
                  />
                </SettingRow>
              )}

              {access.enabled && (
                <SettingRow label="Listening on">
                  <span className="font-mono ui-text-sm break-all text-muted-foreground">
                    {access.reachable.length > 0 ? access.reachable.join(', ') : access.address}
                  </span>
                </SettingRow>
              )}

              {!remote && (
                <SettingRow
                  label="Listen address"
                  description={
                    access.enabled
                      ? 'Turn connections off to change it.'
                      : 'Address and port. 0.0.0.0 listens on every network this computer is on.'
                  }
                >
                  <Input
                    size="sm"
                    value={access.enabled ? access.address : address}
                    onChange={(e) => setDraft(e.target.value)}
                    disabled={access.enabled || busy}
                    autoCapitalize="off"
                    autoCorrect="off"
                    spellCheck={false}
                    className={`${SETTINGS_CONTROL_WIDTHS.textWide} font-mono`}
                  />
                </SettingRow>
              )}

              {error && (
                <p role="alert" className="ui-font ui-text-sm px-1 text-destructive">
                  {error}
                </p>
              )}
            </>
          )}
        </Section>
      </div>

      {view.load.status === 'ready' && access && (
        <Section title="Paired devices">
          {view.devices.length === 0 ? (
            <p className="ui-font ui-text-sm px-1 py-2 text-muted-foreground">
              No devices paired yet.
            </p>
          ) : (
            <div className="space-y-1">
              {view.devices.map((device) => (
                <DeviceRow
                  key={device.id}
                  device={device}
                  isThisComputer={device.id === thisComputerId}
                  onRevoke={(d) => void revoke(d)}
                />
              ))}
            </div>
          )}

          <SettingRow
            label="Pairing"
            description={
              access.enabled
                ? 'Shows a one-time code to enter on the device.'
                : remote
                  ? 'Device connections are off on that computer.'
                  : 'Turn on device connections first.'
            }
          >
            <Button
              type="button"
              variant="outline"
              size="sm"
              aria-label="Add a device"
              disabled={!access.enabled}
              onClick={() => {
                newCode()
              }}
            >
              <Plus />
              Add a device
            </Button>
          </SettingRow>
          <AddDeviceDialog minted={minted} onNewCode={newCode} onClose={closeOffer} />
        </Section>
      )}
    </div>
  )
}
