import { beforeEach, describe, expect, it, vi } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'
import { hydrateSidebar } from '@/lib/persistence/hydrate'
import { success } from '@/lib/loadable'
import { getVisibleProjectIds } from '@/lib/store/project-visibility'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { useSidebarStore, getInitialState } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

vi.mock('@/features/file-system/controllers/platform', () => ({ readWorkspaceFile: vi.fn() }))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId: () => null,
  getHomeOwningChatId: () => null,
}))

const project = (id: string): Project => ({
  id,
  name: id,
  path: `/p/${id}`,
  lastActivity: new Date('2026-09-01T00:00:00Z'),
})

// REGRESSION (restyle v2), same class as the retired `collapsedRepos` gate: the
// pre-restyle tree wrote `collapsedProjects` (project-home-row.tsx /
// workspace-tree.tsx called `toggleProject`), and `getVisibleProjectIds` still
// drops every project in that set — but the restyled sidebar has NO writer for
// it (`toggleProject` has zero callers; the SpaceHeader fold is panel-local),
// so a project the old build folded away loses its repos/home streams and its
// rows whenever it is not the active one, and nothing in the UI can ever bring
// them back. The persisted key must replay as "every project open".
describe('a project the OLD build persisted as collapsed', () => {
  beforeEach(() => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    useSidebarStore.setState(getInitialState())
    useProjectDataStore.setState({ data: success([project('p1'), project('p2')]) })
    useProjectStore.setState({ activeProjectId: 'p1' })
  })

  it('is still visible after hydration — the restyled tree cannot un-fold it', async () => {
    await saveSidebarUI({ collapsedProjects: ['p2'], collapsedChatRows: [] })
    await hydrateSidebar()
    expect(getVisibleProjectIds().has('p2')).toBe(true)
  })

  it('stays visible once the user swipes to another project', async () => {
    await saveSidebarUI({ collapsedProjects: ['p2'], collapsedChatRows: [] })
    await hydrateSidebar()
    useProjectStore.setState({ activeProjectId: 'p2' })
    expect(getVisibleProjectIds().has('p2')).toBe(true)
    useProjectStore.setState({ activeProjectId: 'p1' })
    expect(getVisibleProjectIds().has('p2')).toBe(true)
  })
})
