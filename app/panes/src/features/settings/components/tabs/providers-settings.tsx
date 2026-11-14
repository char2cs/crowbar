import { useState } from 'react'
import { listGlobalProviders, uploadDescriptor } from '@/features/agent/api/agent-providers-api'
import { DescriptorStatus } from '@/features/agent/components/descriptor-status'
import { useProvidersSettings } from '@/features/settings/hooks/use-providers-settings'
import {
  beginProviderWrite,
  isLatestProviderWrite,
  useAgentProvidersStore,
} from '@/features/settings/stores/agent-providers-store'
import { toast } from '@/features/window/stores/toast-store'
import { report } from '@/lib/errors/report'
import { ChatPresentationSetting } from './chat-presentation-setting'
import { DefaultPermissionLevelSetting } from './default-permission-level-setting'
import { ProvidersSettingsView } from './providers-settings-view'

// "Agents" is only the user-facing name ("agent" is overloaded internally); code and wire say
// provider. Edits the local daemon's list from anywhere, and PUTs to the server rather than
// `updateSetting`'s localStorage, since provider state is server-owned.
export const ProvidersSettings = () => {
  const { providers, status, retry, toggle, toggleTools, reorder } = useProvidersSettings()
  const [uploading, setUploading] = useState(false)
  const [descriptorRefresh, setDescriptorRefresh] = useState(0)

  const handleDescriptorUpload = async (file: File) => {
    setUploading(true)
    try {
      const added = await uploadDescriptor(file)
      const seq = beginProviderWrite()
      setDescriptorRefresh((n) => n + 1)
      toast.success('Agent added', `${added.id} is now available as a provider.`)
      try {
        const resolved = await listGlobalProviders()
        if (isLatestProviderWrite(seq)) useAgentProvidersStore.getState().setProviders(resolved)
      } catch {
        if (isLatestProviderWrite(seq)) useAgentProvidersStore.getState().markUnavailable()
        toast.error('Could not refresh agents', 'Reopen this tab to load the new provider.')
      }
    } catch (error) {
      report(error, { action: 'Could not add agent' })
    } finally {
      setUploading(false)
    }
  }

  return (
    <div className="space-y-4">
      <ProvidersSettingsView
        providers={providers}
        status={status}
        onToggle={toggle}
        onToggleTools={toggleTools}
        onReorder={reorder}
        onRetry={retry}
        permission={<DefaultPermissionLevelSetting />}
        descriptor={
          <>
            <label className="ui-font ui-text-sm mx-1 inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-foreground">
              {uploading ? 'Adding agent…' : 'Upload agent descriptor'}
              <input
                type="file"
                accept=".yaml,.yml,application/yaml,text/yaml"
                className="sr-only"
                disabled={uploading}
                onChange={(event) => {
                  const file = event.currentTarget.files?.[0]
                  event.currentTarget.value = ''
                  if (file) void handleDescriptorUpload(file)
                }}
              />
            </label>
            <p className="ui-font ui-text-xs px-1 text-muted-foreground">
              Upload a YAML descriptor with a unique provider ID. Its CLI must be installed
              separately.
            </p>
          </>
        }
        footer={<DescriptorStatus key={descriptorRefresh} />}
      />
      <ChatPresentationSetting />
    </div>
  )
}
