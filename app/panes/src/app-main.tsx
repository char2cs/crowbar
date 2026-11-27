import '@/lib/transport/polyfill'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { enableMapSet } from 'immer'
import { MotionConfig } from 'framer-motion'
import { RouterProvider, createRouter, createHashHistory } from '@tanstack/react-router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { PrimitiveDialogProvider } from '@/components/ui/primitive-dialog-service'
import { routeTree } from './routeTree.gen'
import { connectDaemonEvents } from '@/lib/events/connect'
import { initializeSettingsStore } from '@/features/settings/store'
import { startSettingsStorageSync } from '@/features/settings/lib/settings-storage-sync'
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'
import { installPerfObserver, perfEnabled, pushPerfEntry } from '@/lib/perf/instrumentation'
import { prefetchEditorChunks } from '@/features/panes/components/prefetch-editor-chunks'
import { hydrateCriticalStores } from '@/lib/boot'
import { startHostStatus } from '@/features/hosts/lib/host-status'
import { installGlobalReporting, report } from '@/lib/errors/report'
import { startStream } from '@/core/stream/stream'
import { followChatLifecycle } from '@/features/agent/stores/chat-lifecycle'
import { followWorkspaceScopes } from '@/lib/follow-workspace-scopes'
import '@/core/entities/entity-store'
import { IdbViewedOwner } from '@/features/git/lib/idb-viewed-owner'
import { installViewedOwner } from '@/features/git/lib/viewed-owner'
import { installAppEditor } from '@/features/editor/host/app-editor'

installViewedOwner(new IdbViewedOwner())
installAppEditor()

installPerfObserver()

const uninstallGlobalReporting = installGlobalReporting()
import.meta.hot?.dispose(uninstallGlobalReporting)

const stopStream = startStream()
const stopFollowingChats = followChatLifecycle()
const stopFollowingScopes = followWorkspaceScopes()
if (import.meta.hot) {
  import.meta.hot.dispose(() => {
    stopFollowingChats()
    stopFollowingScopes()
    stopStream()
  })
}

enableMapSet()

const stopHostStatus = startHostStatus()
if (import.meta.hot) import.meta.hot.dispose(() => stopHostStatus())

const disconnectDaemonEvents = connectDaemonEvents()
import.meta.hot?.dispose(disconnectDaemonEvents)

initializeIconThemes()

void initializeSettingsStore()

const stopSettingsStorageSync = startSettingsStorageSync()
if (import.meta.hot) import.meta.hot.dispose(() => stopSettingsStorageSync())

prefetchEditorChunks()

if (import.meta.env.DEV && localStorage.getItem('crowbar:react-scan') === '1') {
  void import('react-scan').then(({ scan }) => scan({ enabled: true, log: false }))
}

if (perfEnabled()) {
  void import('web-vitals').then(({ onINP }) => {
    onINP(
      (m) => {
        pushPerfEntry({
          name: `INP:${m.rating}`,
          startTime: m.entries[0]?.startTime ?? 0,
          duration: m.value,
          entryType: 'inp',
        })
      },
      { reportAllChanges: true },
    )
  })
}

const router = createRouter({ routeTree, history: createHashHistory() })

function renderApp() {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <MotionConfig reducedMotion="user">
        <TooltipProvider>
          <PrimitiveDialogProvider>
            <RouterProvider router={router} />
          </PrimitiveDialogProvider>
        </TooltipProvider>
      </MotionConfig>
    </StrictMode>,
  )
}

hydrateCriticalStores()
  .catch((err: unknown) => report(err, { action: 'Restore the last session' }))
  .finally(renderApp)
