import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { initializeSettingsStore } from '@/features/settings/store'
import { startSettingsStorageSync } from '@/features/settings/lib/settings-storage-sync'
import { MobileFilesPage } from '@/features/window/components/mobile-files-page'

initializeIconThemes()
void initializeSettingsStore()
startSettingsStorageSync()
document.documentElement.setAttribute('data-chrome-page', '')
document.body.classList.add('bg-chrome-bg')

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <MobileFilesPage />
  </StrictMode>,
)
