import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { TooltipProvider } from '@/components/ui/tooltip'
import { initializeSettingsStore } from '@/features/settings/store'
import { startSettingsStorageSync } from '@/features/settings/lib/settings-storage-sync'
import { MobileSidebarPage } from '@/features/window/components/mobile-sidebar-page'

void initializeSettingsStore()
startSettingsStorageSync()
document.documentElement.setAttribute('data-chrome-page', '')
document.body.classList.add('bg-chrome-bg')

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <TooltipProvider>
      <MobileSidebarPage />
    </TooltipProvider>
  </StrictMode>,
)
