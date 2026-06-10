const getVersion = async (): Promise<string> => '0.0.0' // stub in web mode
import { create } from 'zustand'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import type { UpdateInfo } from '../hooks/use-updater'
import {
  buildWhatsNewMarkdown,
  hydrateWhatsNew,
  queuePendingWhatsNew,
  type WhatsNewInfo,
} from '../lib/whats-new'

interface WhatsNewState {
  initialized: boolean
  info: WhatsNewInfo | null
  initialize: () => Promise<void>
  open: () => Promise<void>
  openInfo: (info: WhatsNewInfo) => void
  queuePendingUpdate: (updateInfo: UpdateInfo) => void
}

function openWhatsNewBuffer(info: WhatsNewInfo) {
  const path = `whats-new://v${info.version}.md`
  const name = `What's New ${info.version}.md`
  const content = buildWhatsNewMarkdown(info)

  getActiveWorkspaceStoreRef()?.getState().bufferActions.openContent({
    type: 'markdownPreview',
    path,
    name,
    content,
    sourceFilePath: path,
  })
}

export const useWhatsNewStore = create<WhatsNewState>()((set, get) => ({
  initialized: false,
  info: null,

  initialize: async () => {
    if (get().initialized) {
      return
    }

    const currentVersion = await getVersion()
    const { info, shouldAutoOpen } = hydrateWhatsNew(currentVersion)

    set({
      initialized: true,
      info,
    })

    if (shouldAutoOpen) {
      openWhatsNewBuffer(info)
    }
  },

  open: async () => {
    if (!get().initialized) {
      await get().initialize()
    }

    const info = get().info
    if (!info) {
      return
    }

    openWhatsNewBuffer(info)
  },

  openInfo: (info) => {
    openWhatsNewBuffer(info)
  },

  queuePendingUpdate: (updateInfo) => {
    queuePendingWhatsNew({
      version: updateInfo.version,
      previousVersion: updateInfo.currentVersion,
      body: updateInfo.body,
      date: updateInfo.date,
    })
  },
}))
