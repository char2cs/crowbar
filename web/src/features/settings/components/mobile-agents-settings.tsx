import { useEffect, useState } from 'react'
import type { PermissionLevel } from '@/features/agent/api/agent-api'
import { PermissionLevelRow } from '@/features/settings/components/tabs/default-permission-level-setting'
import { ProvidersSettingsView } from '@/features/settings/components/tabs/providers-settings-view'
import type { NativeAgentsSettings } from '@/lib/native-chrome'
import { postToShell, receiveShellSnapshots } from '@/lib/shell-page-bridge'

const send = (intent: Parameters<typeof postToShell>[1]) => postToShell('agents', intent)
const setLevel = (level: PermissionLevel | null) => {
  if (level) send({ type: 'setDefaultPermission', level })
}

/** The Agents tab drawn from the main webview's snapshot of one host's agents; every edit goes
 *  back to it as an intent, and the next snapshot shows the result. */
export function MobileAgentsSettings() {
  const [snapshot, setSnapshot] = useState<NativeAgentsSettings | null>(null)
  useEffect(() => receiveShellSnapshots<NativeAgentsSettings>('agents', setSnapshot), [])
  return (
    <ProvidersSettingsView
      providers={snapshot?.providers ?? []}
      status={snapshot?.status ?? 'loading'}
      onToggle={(id, enabled) => send({ type: 'toggleProvider', id, enabled })}
      onToggleTools={(id, enabled) => send({ type: 'toggleProviderTools', id, enabled })}
      onReorder={(ids) => send({ type: 'reorderProviders', ids })}
      permission={
        snapshot && <PermissionLevelRow level={snapshot.permissionLevel} onChange={setLevel} />
      }
    />
  )
}
