import { describe, expect, it, beforeEach } from 'vitest'

import { visibleRepos } from '@/components/sidebar/lib/drop-actions'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import {
  getInitialRemovalState,
  useRemovalTrayStore,
  type RemovalDraft,
} from '@/lib/store/sidebar-removal'

/**
 * A folder held for removal transforms IN PLACE (sidebar-row.tsx's
 * `RemovingSidebarRow`) and keeps its children nested under it for the whole
 * eight-second hold — `descendantHiddenIds` deliberately holds the entry's own
 * primary row back so there is a row left to transform.
 *
 * `visibleRepos` read the tray store's RAW `hiddenIds` instead, which does
 * include the folder itself, so `applyPendingRemovals` re-homed the folder's
 * children to the folder's parent. Every sibling index a drop computed during
 * a hold was then counted over a list that did not match the one on screen.
 */

const repo: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'repo-alpha',
  avatarLabel: 'R',
  avatarColor: 'c',
  defaultWorkspaceId: 'ws-main',
  defaultBranch: 'main',
  workspaces: [
    { id: 'ws-inside', branch: 'feat/a', age: '', order: 0, status: 'new', folderId: 'folder-1' },
  ],
  folders: [
    { id: 'folder-1', repoId: 'r1', name: 'repofolder-RN', order: 0 },
    { id: 'folder-child', repoId: 'r1', name: 'nested', order: 0, parentId: 'folder-1' },
  ],
}

const heldFolder: RemovalDraft = {
  kind: 'folder',
  id: 'folder-1',
  label: 'repofolder-RN',
  projectId: 'p1',
  repoId: 'r1',
  wsId: '',
  providerIcon: '',
  hiddenIds: ['folder-1'],
  extra: 0,
  fallbackWsId: null,
}

beforeEach(() => {
  useSidebarStore.setState({ ...getInitialState(), repos: [repo] })
  useRemovalTrayStore.setState(getInitialRemovalState())
})

describe('the drop planner while a folder is held for removal', () => {
  it('keeps the held folder and leaves its children under it', () => {
    useRemovalTrayStore.getState().hold([heldFolder])
    const [visible] = visibleRepos()
    expect(visible.folders?.map((f) => f.id)).toEqual(['folder-1', 'folder-child'])
    expect(visible.folders?.find((f) => f.id === 'folder-child')?.parentId).toBe('folder-1')
    expect(visible.workspaces.find((w) => w.id === 'ws-inside')?.folderId).toBe('folder-1')
  })

  // The cascade half is untouched: a held CHAT's own threads still disappear
  // outright, so this is not "stop filtering anything".
  it('still hides a held row’s cascade descendants', () => {
    useSidebarStore.setState({
      repos: [
        {
          ...repo,
          chats: [
            { id: 'chat-1', title: 'parent', repoId: 'r1', order: 0 },
            { id: 'chat-2', title: 'thread', repoId: 'r1', order: 1, parentId: 'chat-1' },
          ],
        },
      ],
    })
    useRemovalTrayStore.getState().hold([
      {
        ...heldFolder,
        kind: 'chat',
        id: 'chat-1',
        label: 'parent',
        hiddenIds: ['chat-1', 'chat-2'],
        extra: 1,
      },
    ])
    const [visible] = visibleRepos()
    expect(visible.chats?.map((c) => c.id)).toEqual(['chat-1'])
  })

  it('hands back the untouched repos when nothing is held', () => {
    expect(visibleRepos()[0]).toBe(repo)
  })
})
