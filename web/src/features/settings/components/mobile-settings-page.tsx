import { lazy, Suspense, useEffect, useState } from 'react'
import type { SettingsTab } from '@/features/window/stores/ui-state-store'
import { useSurfaceThemeColor } from '@/features/window/hooks/use-surface-theme-color'
import { receiveShellSnapshots } from '@/lib/shell-page-bridge'

// Each tab loads when first opened, so the sheet boots without the others' code.
const PHONE_TABS = {
  appearance: lazy(() =>
    import('./tabs/appearance-settings').then((m) => ({ default: m.AppearanceSettings })),
  ),
  terminal: lazy(() =>
    import('./tabs/terminal-settings').then((m) => ({ default: m.TerminalSettings })),
  ),
  git: lazy(() => import('./tabs/git-settings').then((m) => ({ default: m.GitSettings }))),
  // No IPC and no daemon here: Agents draws the main webview's snapshot.
  providers: lazy(() =>
    import('./mobile-agents-settings').then((m) => ({ default: m.MobileAgentsSettings })),
  ),
} satisfies Partial<Record<SettingsTab, unknown>>

function PhoneTabContent({ tab }: { tab: SettingsTab }) {
  const Tab = PHONE_TABS[tab as keyof typeof PHONE_TABS]
  return (
    Tab && (
      <Suspense>
        <Tab />
      </Suspense>
    )
  )
}

/** One desktop settings tab for a screen the phone's native Settings pushes; the title, back
 *  button and list that lead here are native. Blank until the shell names the tab. */
export function MobileSettingsPage() {
  useSurfaceThemeColor()
  const [tab, setTab] = useState<SettingsTab | null>(null)
  useEffect(() => receiveShellSnapshots<SettingsTab | null>('tab', setTab), [])
  return (
    <main className="flex h-dvh flex-col overflow-hidden bg-background pb-[env(safe-area-inset-bottom)] text-foreground [--settings-section-title:var(--ui-text-lg)]">
      <div
        data-settings-content=""
        className="min-h-0 flex-1 overflow-y-auto px-4 py-3 [--app-ui-control-font-size:var(--ui-text-sm)] [overscroll-behavior:contain]"
      >
        {tab && <PhoneTabContent tab={tab} />}
      </div>
    </main>
  )
}
