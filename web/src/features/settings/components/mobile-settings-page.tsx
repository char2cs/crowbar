import { lazy, Suspense, useState } from 'react'
import type { SettingsTab } from '@/features/window/stores/ui-state-store'
import { useSurfaceThemeColor } from '@/features/window/hooks/use-surface-theme-color'
import { SettingsPanels, SettingsSearchInput, SettingsTitle } from './settings-layout'
import { PHONE_SETTINGS_TAB_ITEMS } from './settings-tab-items'

// Each tab loads when first opened, so the sheet boots without the others' code.
const PHONE_TABS = {
  appearance: lazy(() =>
    import('./tabs/appearance-settings').then((m) => ({ default: m.AppearanceSettings })),
  ),
  terminal: lazy(() =>
    import('./tabs/terminal-settings').then((m) => ({ default: m.TerminalSettings })),
  ),
  git: lazy(() => import('./tabs/git-settings').then((m) => ({ default: m.GitSettings }))),
  // No IPC and no daemon here: Agents and Hosts draw the main webview's snapshots.
  providers: lazy(() =>
    import('./mobile-agents-settings').then((m) => ({ default: m.MobileAgentsSettings })),
  ),
  hosts: lazy(() =>
    import('./mobile-hosts-settings').then((m) => ({ default: m.MobileHostsSettings })),
  ),
} satisfies Partial<Record<SettingsTab, unknown>>

function PhoneTabContent({ tab }: { tab: SettingsTab }) {
  const Tab = PHONE_TABS[tab as keyof typeof PHONE_TABS] ?? PHONE_TABS.appearance
  return (
    <Suspense>
      <Tab />
    </Suspense>
  )
}

/** The desktop settings dialog's content for the phone's native settings sheet, without the
 *  dialog's frame: at this width its own CSS puts the tabs in the title's dropdown. */
export function MobileSettingsPage() {
  useSurfaceThemeColor()
  const [activeTab, setActiveTab] = useState<SettingsTab>('appearance')
  const tabs = { tabs: PHONE_SETTINGS_TAB_ITEMS, activeTab, onTabChange: setActiveTab }
  return (
    <main className="flex h-dvh flex-col overflow-hidden bg-background pb-[env(safe-area-inset-bottom)] text-foreground">
      <header className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 px-3 pt-2 pb-1">
        <div className="font-heading min-w-0 text-lg">
          <SettingsTitle {...tabs} />
        </div>
        <SettingsSearchInput />
      </header>
      <div className="flex min-h-0 flex-1">
        <SettingsPanels {...tabs}>
          <PhoneTabContent tab={activeTab} />
        </SettingsPanels>
      </div>
    </main>
  )
}
