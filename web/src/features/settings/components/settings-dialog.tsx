import { useEffect, useState } from 'react'
import { useSettingsStore } from '@/features/settings/store'
import { type SettingsTab, useUIState } from '@/features/window/stores/ui-state-store'
import { AppDialog as Dialog } from '@/components/ui/dialog'
import { SETTINGS_TAB_ITEMS } from './settings-tab-items'
import { SettingsPanels, SettingsSearchInput, SettingsTitle } from './settings-layout'
import { AppearanceSettings } from './tabs/appearance-settings'
import { EditorSettings } from './tabs/editor-settings'
import { GitSettings } from './tabs/git-settings'
import { FileTreeSettings } from './tabs/file-tree-settings'
import { TerminalSettings } from './tabs/terminal-settings'
import { BrowserSettings } from './tabs/browser-settings'
import { ProvidersSettings } from './tabs/providers-settings'
import { DeveloperSettings } from './tabs/developer-settings'
import { KeybindingsSettings } from './tabs/keybindings-settings'
import { ActiveSessionsSettings } from './tabs/active-sessions-settings'
import { HostsSettings } from './tabs/hosts-settings'

interface SettingsDialogProps {
  isOpen: boolean
  onClose: () => void
}

const SettingsDialog = ({ isOpen, onClose }: SettingsDialogProps) => {
  const settingsInitialTab = useUIState((s) => s.settingsInitialTab)
  const setSettingsInitialTab = useUIState((s) => s.setSettingsInitialTab)
  const [activeTab, setActiveTab] = useState<SettingsTab>('appearance')

  const clearSearch = useSettingsStore((state) => state.clearSearch)
  // Sync active tab with settingsInitialTab whenever it changes (enables deep linking when dialog is already open)
  useEffect(() => {
    if (isOpen) {
      setActiveTab(settingsInitialTab)
    }
  }, [settingsInitialTab, isOpen])

  // Remember the last active tab so it persists across open/close
  const handleTabChange = (tab: SettingsTab) => {
    setActiveTab(tab)
    setSettingsInitialTab(tab)
  }

  // Clear search when the dialog closes or unmounts (the shell mounts it only while open)
  useEffect(() => {
    if (isOpen) return clearSearch
  }, [isOpen, clearSearch])

  if (!isOpen) return null

  const tabs = { tabs: SETTINGS_TAB_ITEMS, activeTab, onTabChange: handleTabChange }
  return (
    <Dialog
      onClose={onClose}
      title={<SettingsTitle {...tabs} />}
      headerActions={<SettingsSearchInput className="w-64 max-[720px]:w-44 max-[520px]:w-32" />}
      classNames={{
        modal:
          'h-[74vh] max-h-[820px] w-[90vw] max-w-[1120px] min-w-0 border-0 max-[720px]:h-[86vh] max-[720px]:w-[calc(100vw-32px)] [&>div:first-child]:border-b-0',
        header: 'max-[720px]:grid max-[720px]:grid-cols-[minmax(0,1fr)_auto] max-[720px]:gap-2',
        title: 'max-[720px]:min-w-0',
        headerActions: 'max-[720px]:min-w-0',
        content: 'flex p-0',
      }}
    >
      <SettingsPanels {...tabs}>
        <SettingsTabContent tab={activeTab} />
      </SettingsPanels>
    </Dialog>
  )
}

/** The desktop tab `tab`. */
function SettingsTabContent({ tab }: { tab: SettingsTab }) {
  switch (tab) {
    case 'appearance':
      return <AppearanceSettings />
    case 'editor':
      return <EditorSettings />
    case 'file-explorer':
      return <FileTreeSettings />
    case 'git':
      return <GitSettings />
    case 'terminal':
      return <TerminalSettings />
    case 'browser':
      return <BrowserSettings />
    case 'providers':
      return <ProvidersSettings />
    case 'keybindings':
      return <KeybindingsSettings />
    case 'active-sessions':
      return <ActiveSessionsSettings />
    case 'hosts':
      return <HostsSettings />
    case 'developer':
      return <DeveloperSettings />
    default:
      return <AppearanceSettings />
  }
}

export default SettingsDialog
