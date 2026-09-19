// §8 of the project-scoped panes design: no migration, one branch at hydrate.
// A record written before views carried a project is READ correctly — its
// views are filed from their own chats where that is answerable, and left
// untagged (for the first `setActiveProject` to adopt) where it is not.
import { describe, it, expect } from 'vitest'
import {
  restoreViewProjects,
  restoreActiveViewByProject,
  type RestoredWindowViews,
} from '@/lib/persistence/hydrate'
import type { PaneGroup } from '@/features/panes/types/pane'

function pane(id: string, viewId: string, chatId: string | null): PaneGroup {
  return {
    id,
    type: 'group',
    chatId,
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
    chatSelected: true,
    viewId,
  }
}

const views: RestoredWindowViews = {
  rootLayout: { type: 'pane', id: 'pane-1' },
  parkedViews: { 'view-b': { type: 'pane', id: 'pane-2' } },
  activeViewId: 'view-a',
  activePaneId: 'pane-1',
}

const panes = {
  'pane-1': pane('pane-1', 'view-a', 'chat-a'),
  'pane-2': pane('pane-2', 'view-b', 'chat-b'),
}

const projectOfChat: Record<string, string> = { 'chat-a': 'project-a', 'chat-b': 'project-b' }
const resolve = (chatId: string) => projectOfChat[chatId] ?? null

describe('restoreViewProjects', () => {
  it('derives each view’s project from its own panes’ chats', () => {
    expect(restoreViewProjects({ panes }, views, resolve)).toEqual({
      'view-a': 'project-a',
      'view-b': 'project-b',
    })
  })

  it('a record that already carries the tag is trusted over the derivation', () => {
    const tagged = { panes, viewProjects: { 'view-a': 'project-z' } }

    expect(restoreViewProjects(tagged, views, resolve)).toEqual({
      'view-a': 'project-z',
      'view-b': 'project-b',
    })
  })

  it('an unresolvable view is left UNTAGGED rather than refused', () => {
    // Adoption, not a crash: the first `setActiveProject` files it. A
    // mis-filed view costs one gesture; an unreachable one costs the chat.
    expect(restoreViewProjects({ panes }, views, () => null)).toEqual({})
  })

  it('a chatless view (the empty stage) resolves to nothing and stays untagged', () => {
    const empty = { 'pane-1': pane('pane-1', 'view-a', null) }

    expect(restoreViewProjects({ panes: empty }, views, resolve)).toEqual({})
  })

  it('never files a view this window no longer holds', () => {
    const stale = { panes, viewProjects: { 'view-gone': 'project-a' } }

    expect(restoreViewProjects(stale, views, resolve)['view-gone']).toBeUndefined()
  })
})

describe('restoreActiveViewByProject', () => {
  it('keeps the pointers whose views came back', () => {
    const layout = { panes, activeViewByProject: { 'project-b': 'view-b' } }

    expect(restoreActiveViewByProject(layout, views)).toEqual({ 'project-b': 'view-b' })
  })

  it('drops a pointer to a view this window no longer holds', () => {
    // A stale pointer sends the project switch at a view that is not there,
    // landing it on the empty stage instead of on that project's real content.
    const layout = { panes, activeViewByProject: { 'project-b': 'view-closed' } }

    expect(restoreActiveViewByProject(layout, views)).toEqual({})
  })

  it('a record written before the partition existed replays as nothing', () => {
    expect(restoreActiveViewByProject({ panes }, views)).toEqual({})
  })
})
