import '@/lib/transport/polyfill'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { enableMapSet } from 'immer'
import { RouterProvider, createRouter, createHashHistory } from '@tanstack/react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { TooltipProvider } from '@/components/ui/tooltip'
import { routeTree } from './routeTree.gen'
import { queryClient } from '@/lib/queries/client'
import { connectDaemonEvents } from '@/lib/events/connect'
import { initializeSettingsStore } from '@/features/settings/store'
import { ensureStartupAppearanceApplied } from '@/features/settings/lib/appearance-bootstrap'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { initTreeCacheSubscription } from '@/features/editor/stores/tree-cache-store'
import { initViewStoreSubscription } from '@/features/editor/stores/view-store'
import './index.css'

// Required for Zustand stores that use immer middleware with Set/Map state
enableMapSet()

// Wire up daemon event listeners for cache invalidation.
// This must be called once at module load, before any queries are made.
// Store disconnect function (used in tests and future hot reload)
connectDaemonEvents(queryClient)

// Apply the cached theme immediately (synchronous) so the correct dark/light
// class is set before React renders anything — prevents a flash of light mode.
ensureStartupAppearanceApplied()

// Register all built-in icon themes with the registry before React renders
// so the icon theme dropdown in settings is populated when first opened
initializeIconThemes()

// Kick off settings load. The store starts with defaults and updates
// asynchronously when localStorage values are loaded — this fires the
// subscriptions in settings-store.ts which propagate to editor/theme/etc.
void initializeSettingsStore()

// Wire up tree-sitter cache cleanup: removes parse trees when buffers are closed.
initTreeCacheSubscription()
initViewStoreSubscription()

const router = createRouter({ routeTree, history: createHashHistory() })

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>,
)
