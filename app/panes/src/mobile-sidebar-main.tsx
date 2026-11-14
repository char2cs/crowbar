import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { TooltipProvider } from '@/components/ui/tooltip'
import { initializeSettingsStore } from '@/features/settings/store'
import { startSettingsStorageSync } from '@/features/settings/lib/settings-storage-sync'
import { MobileSidebarPage } from '@/features/window/components/mobile-sidebar-page'

// Theme and fonts from the cache the app keeps in the shared localStorage, then live from the
// settings the other webviews save there; read only.
void initializeSettingsStore()
startSettingsStorageSync()
// The page's surface on body too: its theme-color is read from it.
// Crowbar paints this page's chrome dark in either mode (crowbar.css).
document.documentElement.setAttribute('data-chrome-page', '')
document.body.classList.add('bg-chrome-bg')

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <TooltipProvider>
      <MobileSidebarPage />
    </TooltipProvider>
  </StrictMode>,
)
