import { assetURL } from '@/lib/api'
import { qualify } from '@/lib/host-path'
import { hostIdForProject } from '@/lib/host-resolve'

function projectPath(projectId: string): string {
  return `/v0/projects/${encodeURIComponent(projectId)}`
}

export function projectBase(projectId: string): string {
  return qualify(hostIdForProject(projectId), projectPath(projectId))
}

export function repoBase(projectId: string, repoId: string): string {
  const path = `${projectPath(projectId)}/repos/${encodeURIComponent(repoId)}`
  return qualify(hostIdForProject(projectId), path)
}

export function projectAssetURL(projectId: string, path: string): string {
  return assetURL(qualify(hostIdForProject(projectId), path))
}
