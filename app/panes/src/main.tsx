import './index.css'
import { ensureStartupAppearanceApplied } from '@/features/settings/lib/appearance-bootstrap'

// With the stylesheet, before any page's chunk loads: the first paint is the stored theme
// on this platform's opaque surfaces, never the default light, translucent wash.
ensureStartupAppearanceApplied()

// The phone's chat list, settings sheet and file drawer are extra webviews with no IPC: each
// boots only its own page, never the app (no daemon connection, no store hydration).
const hash = window.location.hash
if (hash.startsWith('#/mobile-sidebar')) void import('./mobile-sidebar-main')
else if (hash.startsWith('#/mobile-settings')) void import('./mobile-settings-main')
else if (hash.startsWith('#/mobile-files')) void import('./mobile-files-main')
else void import('./app-main')
