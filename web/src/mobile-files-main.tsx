import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { initializeSettingsStore } from '@/features/settings/store'
import { startSettingsStorageSync } from '@/features/settings/lib/settings-storage-sync'
import { MobileFilesPage } from '@/features/window/components/mobile-files-page'

// Theme, fonts and icon theme from the settings the app keeps in the shared localStorage; read only.
initializeIconThemes()
void initializeSettingsStore()
startSettingsStorageSync()
// The page's surface on body too: its theme-color is read from it.
// Crowbar paints this page's chrome dark in either mode (crowbar.css).
document.documentElement.setAttribute('data-chrome-page', '')
document.body.classList.add('bg-chrome-bg')

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <MobileFilesPage />
  </StrictMode>,
)
