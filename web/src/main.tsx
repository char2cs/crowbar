import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { enableMapSet } from 'immer'
import { RouterProvider, createRouter, createHashHistory } from '@tanstack/react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { TooltipProvider } from '@/components/ui/tooltip'
import { routeTree } from './routeTree.gen'
import { queryClient } from './lib/query'
import { initializeSettingsStore } from '@/features/settings/store'
import './index.css'

// Required for Zustand stores that use immer middleware with Set/Map state
enableMapSet()

// Kick off settings load immediately. The store starts with defaults and
// updates asynchronously when localStorage values are loaded — this fires the
// subscriptions in settings-store.ts which propagate to editor/theme/etc.
void initializeSettingsStore()

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
