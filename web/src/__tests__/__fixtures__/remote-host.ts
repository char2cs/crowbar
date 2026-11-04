import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { Project } from '@/lib/types'
import {
  markProjectsSynced,
  resetEntities,
  seedProjects,
} from '@/__tests__/__fixtures__/sidebar-entities'

/** The world of a user who never paired a remote and has not loaded any project. */
export function resetHostWorld(): void {
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  resetEntities()
}

/** Pairs `hostId` through the real store, so `remoteEverPaired` flips like production. */
export function pairRemoteHost(hostId = 'host-1', name = hostId): void {
  useHostConfigStore.getState().addHost({
    id: hostId,
    name,
    endpoint: `${hostId}.example:3737`,
    pairedFingerprint: 'SHA256:aaaa',
    clientId: `client-${hostId}`,
  })
}

export function projectOn(id: string, hostId: string): Project {
  return { id, name: id, path: `/work/${id}`, lastActivity: new Date(0), hostId }
}

/** The projects scopes' snapshots landing, the local daemon's and each named host's: what
 *  makes a paired install's workspaces resolve. */
export function loadProjects(...projects: Project[]): void {
  seedProjects(projects)
  markProjectsSynced()
  for (const hostId of new Set(projects.map((p) => p.hostId))) markProjectsSynced(hostId)
}
