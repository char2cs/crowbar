import { useEffect, useMemo, useState } from 'react'
import { HostViewsContext } from '@/features/hosts/hooks/use-host-status'
import { HostsSettingsView } from '@/features/settings/components/tabs/hosts-settings'
import type { NativeHostsSettings } from '@/lib/native-chrome'
import { postToShell, receiveShellSnapshots } from '@/lib/shell-page-bridge'

/** The Hosts tab drawn from the main webview's snapshot; revoke and Add host go back to it as
 *  intents, and it asks its questions natively over this sheet. */
export function MobileHostsSettings() {
  const [snapshot, setSnapshot] = useState<NativeHostsSettings | null>(null)
  useEffect(() => receiveShellSnapshots<NativeHostsSettings>('hosts', setSnapshot), [])
  const hosts = snapshot?.hosts
  const views = useMemo(
    () => new Map(hosts?.map((h) => [h.id, { name: h.name, state: h.state }])),
    [hosts],
  )
  const revoking = useMemo(() => new Set(snapshot?.revoking), [snapshot?.revoking])
  return (
    <HostViewsContext value={views}>
      <HostsSettingsView
        hosts={hosts ?? []}
        showLocal={false}
        description="Add a machine running the Crowbar daemon to reach it from this phone."
        revoking={revoking}
        onRevoke={(host) => postToShell('hosts', { type: 'revokeHost', id: host.id })}
        onAdd={() => postToShell('hosts', { type: 'addHost' })}
      />
    </HostViewsContext>
  )
}
