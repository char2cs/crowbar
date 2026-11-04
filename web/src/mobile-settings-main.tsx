import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { TooltipProvider } from '@/components/ui/tooltip'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { MobileSettingsPage } from '@/features/settings/components/mobile-settings-page'
import { startSettingsStorageSync } from '@/features/settings/lib/settings-storage-sync'
import { initializeSettingsStore } from '@/features/settings/store'

initializeIconThemes()
void initializeSettingsStore()
startSettingsStorageSync()
// The page's surface on body too: its theme-color is read from it.
document.body.classList.add('bg-background')

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <TooltipProvider>
      <MobileSettingsPage />
    </TooltipProvider>
  </StrictMode>,
)
