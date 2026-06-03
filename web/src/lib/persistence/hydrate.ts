import { getDB } from './idb'
import type { WorkspaceLayout, EditorState, UIPreferences } from './schemas'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useSettingsStore } from '@/features/settings/store'
import { loadSidebarUI } from './sidebar-ui'
import { loadAllWorkspaceHierarchies } from './workspace-hierarchy'
import { useSidebarStore } from '@/lib/store/sidebar'

export interface WorkspaceHydrationResult {
  layout: WorkspaceLayout | null
  editorStates: EditorState[]
}

export async function hydratePreferences(): Promise<UIPreferences | null> {
  const db = await getDB()
  const prefs = await db.get('ui-preferences', 'global').then(r => r ?? null)

  if (prefs) {
    useSettingsStore.setState((state) => ({
      settings: {
        ...state.settings,
        theme: prefs.theme,
        fontSize: prefs.fontSize,
        fontFamily: prefs.fontFamily,
        tabSize: prefs.tabSize,
        wordWrap: prefs.wordWrap,
        showMinimap: prefs.minimap,
      },
    }))
  }

  return prefs
}

export async function hydrateWorkspace(workspaceId: string): Promise<WorkspaceHydrationResult> {
  const db = await getDB()

  const [layout, editorStates] = await Promise.all([
    db.get('workspace-layout', workspaceId).then(r => r ?? null),
    db.getAllFromIndex('editor-state', 'workspaceId', workspaceId),
  ])

  const store = getOrCreateWorkspaceStore(workspaceId)

  if (layout) {
    store.setState({
      activePaneId: layout.activePaneId,
      mostRecentActivePaneIds: layout.mostRecentActivePaneIds ?? [layout.activePaneId],
      panes: layout.panes,
      rootLayout: layout.rootLayout,
      bottomLayout: layout.bottomLayout,
      buffers: layout.buffers ?? [],
    })
  }

  return { layout, editorStates }
}

export async function hydrateSidebar(): Promise<void> {
  const [sidebarUI, hierarchies] = await Promise.all([
    loadSidebarUI(),
    loadAllWorkspaceHierarchies(),
  ])

  if (sidebarUI) {
    useSidebarStore.setState({
      collapsedRepos: new Set(sidebarUI.collapsedRepos),
      collapsedWorkspaces: new Set(sidebarUI.collapsedWorkspaces ?? []),
      collapsedChats: new Set(sidebarUI.collapsedChats ?? []),
    })
  }

  if (hierarchies.length > 0) {
    useSidebarStore.setState(s => ({
      repos: s.repos.map(repo => {
        const hierarchy = hierarchies.find(h => h.repoId === repo.id)
        if (!hierarchy) return repo
        const entryMap = new Map(hierarchy.entries.map(e => [e.wsId, e.parentId]))
        return {
          ...repo,
          workspaces: repo.workspaces.map(ws =>
            entryMap.has(ws.id)
              ? { ...ws, parentId: entryMap.get(ws.id) }
              : ws,
          ),
        }
      }),
    }))
  }
}
