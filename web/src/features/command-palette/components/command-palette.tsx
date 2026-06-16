const appDataDir = async (): Promise<string> => '/tmp/crowbar-data' // stub for web mode
import { ClockCounterClockwise as History } from '@phosphor-icons/react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { IconThemeSelectorContent } from '@/features/command-palette/components/icon-theme-selector'
import { ThemeSelectorContent } from '@/features/command-palette/components/theme-selector'
import { useLspStore } from '@/features/editor/lsp/lsp-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { commitChanges } from '@/features/git/api/git-commits-api'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { GitDiff } from '@/features/git/types/git-types'
import { fetchChanges, pullChanges, pushChanges } from '@/features/git/api/git-remotes-api'
import {
  discardAllChanges,
  stageAllFiles,
  unstageAllFiles,
} from '@/features/git/api/git-status-api'
import { useGitStore } from '@/features/git/stores/git-store'
import { useRepositoryStore } from '@/features/git/stores/git-repository-store'
import { useToast } from '@/features/layout/contexts/toast-context'
import { useSettingsStore } from '@/features/settings/store'
import { useWhatsNewStore } from '@/features/settings/stores/whats-new-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { keymapRegistry } from '@/features/keymaps/utils/registry'
import {
  Command,
  CommandEmpty,
  CommandHeader,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import Keybinding from '@/components/ui/keybinding'
import { matchesSearchQuery } from '@/utils/search-match'
import { createAdvancedActions } from '../constants/advanced-actions'
import { createFileActions } from '../constants/file-actions'
import { createGitActions } from '../constants/git-actions'
import { createMarkdownActions } from '../constants/markdown-actions'
import { createNavigationActions } from '../constants/navigation-actions'
import { createPaneActions } from '../constants/pane-actions'
import { createSettingsActions } from '../constants/settings-actions'
import { createViewActions } from '../constants/view-actions'
import { createWindowActions } from '../constants/window-actions'
import type { Action } from '../models/action.types'
import type { CommandPaletteViewId } from '../models/view.types'
import { useActionsStore } from '../store'

const CommandPalette = () => {
  // Get data from stores
  const isCommandPaletteVisible = useUIState((s) => s.isCommandPaletteVisible)
  const commandPaletteInitialView = useUIState((s) => s.commandPaletteInitialView)
  const setIsCommandPaletteVisible = useUIState((s) => s.setIsCommandPaletteVisible)
  const setIsSettingsDialogVisible = useUIState((s) => s.setIsSettingsDialogVisible)
  const isSidebarVisible = useUIState((s) => s.isSidebarVisible)
  const setIsSidebarVisible = useUIState((s) => s.setIsSidebarVisible)
  const isBottomPaneVisible = useUIState((s) => s.isBottomPaneVisible)
  const setIsBottomPaneVisible = useUIState((s) => s.setIsBottomPaneVisible)
  const bottomPaneActiveTab = useUIState((s) => s.bottomPaneActiveTab)
  const setBottomPaneActiveTab = useUIState((s) => s.setBottomPaneActiveTab)
  const isFindVisible = useUIState((s) => s.isFindVisible)
  const setIsFindVisible = useUIState((s) => s.setIsFindVisible)
  const setActiveView = useUIState((s) => s.setActiveView)
  const setIsQuickOpenVisible = useUIState((s) => s.setIsQuickOpenVisible)
  const openSettingsDialog = useUIState((s) => s.openSettingsDialog)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const isVisible = isCommandPaletteVisible
  const onClose = () => {
    setIsCommandPaletteVisible(false)
    setViewStack(['root'])
  }

  const [query, setQuery] = useState('')
  const [selectedIndex, setSelectedIndex] = useState(0)
  const [viewStack, setViewStack] = useState<CommandPaletteViewId[]>(['root'])
  const inputRef = useRef<HTMLInputElement>(null)
  const resultsRef = useRef<HTMLDivElement>(null)
  const currentView = viewStack[viewStack.length - 1] || 'root'
  const isRootView = currentView === 'root'

  const pushView = (view: CommandPaletteViewId) => {
    setQuery('')
    setSelectedIndex(0)
    setViewStack((currentStack) => [...currentStack, view])
  }

  const popView = () => {
    setViewStack((currentStack) =>
      currentStack.length > 1 ? currentStack.slice(0, -1) : currentStack,
    )
  }

  const handleThemeChange = useCallback((theme: string) => {
    void useSettingsStore.getState().updateSetting('theme', theme)
  }, [])

  const handleIconThemeChange = useCallback((iconTheme: string) => {
    void useSettingsStore.getState().updateSetting('iconTheme', iconTheme)
  }, [])

  const lastEnteredActions = useActionsStore.use.lastEnteredActionsStack()
  const pushAction = useActionsStore.use.pushAction()
  const settings = useSettingsStore((s) => s.settings)
  const updateSetting = useSettingsStore((s) => s.updateSetting)
  const setSettingsSearchQuery = useSettingsStore((s) => s.setSearchQuery)
  const lspStatus = useLspStore.use.lspStatus()
  const { clearLspError, updateLspStatus } = useLspStore.use.actions()
  const rootFolderPath = useFileSystemStore((s) => s.rootFolderPath)
  const activeRepoPath = useRepositoryStore.use.activeRepoPath()
  const gitStoreActions = useGitStore((s) => s.actions)
  const { showToast } = useToast()
  const openWhatsNew = useWhatsNewStore((state) => state.open)
  const workspaceStore = useWorkspaceStore()
  const buffers = useStore(workspaceStore, (s) => s.buffers)
  const activePaneId = useStore(workspaceStore, (s) => s.activePaneId)
  const activeBufferId = useStore(
    workspaceStore,
    (s) => s.paneActions.getActivePane()?.activeBufferId ?? null,
  )
  const activeBuffer = buffers.find((b) => b.id === activeBufferId) || null
  const closeBuffer = (id: string) => workspaceStore.getState().bufferActions.closeBuffer(id)
  const setActiveBuffer = (id: string) =>
    workspaceStore.getState().paneActions.activatePaneBuffer(activePaneId, id)
  const switchToNextBuffer = () => workspaceStore.getState().paneActions.switchToNextBufferInPane()
  const switchToPreviousBuffer = () =>
    workspaceStore.getState().paneActions.switchToPreviousBufferInPane()
  const reopenClosedTab = async () => {
    workspaceStore.getState().bufferActions.reopenLastClosedBuffer()
  }
  const { zoomIn, zoomOut, resetZoom } = useZoomStore.use.actions()
  const openBuffer = (
    path: string,
    name: string,
    content: string,
    _isImage?: boolean,
    _databaseType?: string,
    _isDiff?: boolean,
    _isVirtual?: boolean,
    diffData?: GitDiff | MultiFileDiff,
    isMarkdownPreview?: boolean,
    isHtmlPreview?: boolean,
    isCsvPreview?: boolean,
    sourceFilePath?: string,
  ) => {
    if (isMarkdownPreview) {
      return workspaceStore.getState().bufferActions.openContent({
        type: 'markdownPreview',
        path,
        name,
        content,
        sourceFilePath: sourceFilePath ?? path,
      })
    }
    if (isHtmlPreview) {
      return workspaceStore.getState().bufferActions.openContent({
        type: 'htmlPreview',
        path,
        name,
        content,
        sourceFilePath: sourceFilePath ?? path,
      })
    }
    if (isCsvPreview) {
      return workspaceStore.getState().bufferActions.openContent({
        type: 'csvPreview',
        path,
        name,
        content,
        sourceFilePath: sourceFilePath ?? path,
      })
    }
    if (diffData) {
      return workspaceStore
        .getState()
        .bufferActions.openContent({ type: 'diff', path, name, content, diffData })
    }
    return workspaceStore
      .getState()
      .bufferActions.openContent({ type: 'editor', path, name, content })
  }

  // Helper function to check if the active buffer is a markdown file
  const isMarkdownFile = () => {
    if (!activeBuffer) return false
    const extension = activeBuffer.path.split('.').pop()?.toLowerCase()
    return extension === 'md' || extension === 'markdown'
  }

  // Create all actions using factory functions
  const allActions: Action[] = [
    ...createMarkdownActions({
      isMarkdownFile: isMarkdownFile(),
      activeBuffer,
      openBuffer,
      onClose,
    }),
    ...createViewActions({
      isSidebarVisible,
      setIsSidebarVisible,
      isBottomPaneVisible,
      setIsBottomPaneVisible,
      bottomPaneActiveTab,
      setBottomPaneActiveTab,
      isFindVisible,
      setIsFindVisible,
      settings: {
        sidebarPosition: settings.sidebarPosition,
        nativeMenuBar: settings.nativeMenuBar,
        compactMenuBar: settings.compactMenuBar,
      },
      updateSetting: updateSetting as (key: string, value: unknown) => void | Promise<void>,
      zoomIn,
      zoomOut,
      resetZoom,
      onClose,
    }),
    ...createSettingsActions({
      query,
      settings,
      setIsSettingsDialogVisible,
      openSettingsDialog,
      setSettingsSearchQuery,
      pushPaletteView: pushView,
      updateSetting: updateSetting as (key: string, value: unknown) => void | Promise<void>,
      handleFileOpen,
      getAppDataDir: appDataDir,
      openWhatsNew,
      onClose,
    }),
    ...createNavigationActions({
      setIsSidebarVisible,
      setActiveView,
      setIsQuickOpenVisible,
      openSettingsDialog,
      onClose,
    }),
    ...createPaneActions({
      onClose,
    }),
    ...createFileActions({
      activeBufferId,
      buffers,
      closeBuffer,
      switchToNextBuffer,
      switchToPreviousBuffer,
      setActiveBuffer,
      reopenClosedTab,
      onClose,
    }),
    ...createWindowActions({
      onClose,
    }),
    ...createGitActions({
      rootFolderPath,
      activeRepoPath,
      showToast,
      gitStore: { actions: gitStoreActions },
      gitOperations: {
        stageAllFiles,
        unstageAllFiles,
        commitChanges,
        pushChanges,
        pullChanges,
        fetchChanges,
        discardAllChanges,
      },
      onClose,
    }),
    ...createAdvancedActions({
      lspStatus,
      updateLspStatus: updateLspStatus as (
        status: string,
        workspaces?: string[],
        error?: string,
      ) => void,
      clearLspError,
      rootFolderPath,
      pushPaletteView: pushView,
      showToast,
      onClose,
    }),
  ]

  // Filter actions based on query
  const filteredActions = allActions.filter(
    (action) =>
      !query.trim() ||
      matchesSearchQuery(query, [action.label, action.description ?? '', action.category]),
  )

  const prioritizedActions = useMemo(() => {
    if (!settings.coreFeatures.persistentCommands) return filteredActions
    if (!filteredActions) return []

    const remaining = filteredActions.filter((action) => !lastEnteredActions.includes(action.id))

    const prioritized = lastEnteredActions
      .map((id) => filteredActions.find((a) => a.id === id))
      .filter((a): a is Action => !!a) // Filter out undefined and assure it is of type Action

    return [...prioritized, ...remaining]
  }, [filteredActions, lastEnteredActions, settings.coreFeatures.persistentCommands])

  // Handle keyboard navigation
  useEffect(() => {
    if (!isVisible || !isRootView) return

    const handleKeyDown = (e: KeyboardEvent) => {
      switch (e.key) {
        case 'ArrowDown':
          e.preventDefault()
          setSelectedIndex((prev) => (prev < prioritizedActions.length - 1 ? prev + 1 : prev))
          break
        case 'ArrowUp':
          e.preventDefault()
          setSelectedIndex((prev) => (prev > 0 ? prev - 1 : prev))
          break
        case 'Enter':
          e.preventDefault()
          if (prioritizedActions[selectedIndex]) {
            prioritizedActions[selectedIndex].action()
            pushAction(prioritizedActions[selectedIndex].id)
          }
          break
        // Escape is now handled globally in use-keyboard-shortcuts
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [isVisible, isRootView, selectedIndex, prioritizedActions, pushAction])

  // Reset state when visibility changes
  useEffect(() => {
    if (isVisible) {
      setQuery('')
      setSelectedIndex(0)
      setViewStack(
        commandPaletteInitialView === 'root' || !commandPaletteInitialView
          ? ['root']
          : ['root', commandPaletteInitialView as CommandPaletteViewId],
      )
      requestAnimationFrame(() => {
        if (inputRef.current) {
          inputRef.current.focus()
        }
      })
    }
  }, [isVisible, commandPaletteInitialView])

  // Update selected index when query changes
  useEffect(() => {
    setSelectedIndex(0)
  }, [query])

  // Scroll selected item into view
  useEffect(() => {
    if (resultsRef.current && filteredActions.length > 0) {
      const selectedElement = resultsRef.current.children[selectedIndex] as HTMLElement
      if (selectedElement) {
        selectedElement.scrollIntoView({
          block: 'nearest',
          behavior: 'smooth',
        })
      }
    }
  }, [selectedIndex, filteredActions.length])

  if (!isVisible) return null

  return (
    <Command isVisible={isVisible} onClose={onClose}>
      {currentView === 'color-theme' ? (
        <ThemeSelectorContent
          isActive={currentView === 'color-theme'}
          onBack={popView}
          onClose={onClose}
          onThemeChange={handleThemeChange}
          currentTheme={settings.theme}
        />
      ) : currentView === 'icon-theme' ? (
        <IconThemeSelectorContent
          isActive={currentView === 'icon-theme'}
          onBack={popView}
          onClose={onClose}
          onThemeChange={handleIconThemeChange}
          currentTheme={settings.iconTheme}
        />
      ) : (
        <>
          <CommandHeader
            onClose={onClose}
            showClearButton={settings.coreFeatures.persistentCommands}
          >
            <CommandInput
              ref={inputRef}
              value={query}
              onChange={setQuery}
              placeholder="Type a command..."
            />
          </CommandHeader>

          <CommandList ref={resultsRef}>
            {filteredActions.length === 0 ? (
              <CommandEmpty>No commands found</CommandEmpty>
            ) : (
              prioritizedActions.map((action, index) => {
                const isRecent =
                  settings.coreFeatures.persistentCommands && lastEnteredActions.includes(action.id)
                const binding = action.commandId
                  ? keymapRegistry.getKeybinding(action.commandId)?.key
                  : undefined
                return (
                  <CommandItem
                    key={action.id}
                    onClick={() => {
                      action.action()
                      pushAction(action.id)
                    }}
                    isSelected={index === selectedIndex}
                    className="px-3 py-1.5"
                  >
                    {isRecent && <History className="shrink-0 text-muted-foreground" />}
                    <div className="min-w-0 flex-1">
                      <div className="truncate ui-text-xs">{action.label}</div>
                    </div>
                    {binding && (
                      <div className="shrink-0">
                        <Keybinding binding={binding} />
                      </div>
                    )}
                  </CommandItem>
                )
              })
            )}
          </CommandList>
        </>
      )}
    </Command>
  )
}

CommandPalette.displayName = 'CommandPalette'

export default CommandPalette
