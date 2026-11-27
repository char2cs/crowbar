import './index.css'
import { ensureStartupAppearanceApplied } from '@/features/settings/lib/appearance-bootstrap'

ensureStartupAppearanceApplied()

const hash = window.location.hash
if (hash.startsWith('#/mobile-sidebar')) void import('./mobile-sidebar-main')
else if (hash.startsWith('#/mobile-settings')) void import('./mobile-settings-main')
else if (hash.startsWith('#/mobile-files')) void import('./mobile-files-main')
else void import('./app-main')
