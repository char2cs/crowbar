import { apiFetch } from '@/lib/api'
import { identityBaseForWorkspace } from '@/lib/workspace-scope-url'

export interface IdentityDTO {
  login: string
  displayName: string
  avatarUrl: string
}

/** GET the GitHub/GitLab identity associated with the active workspace's remote. */
export async function getIdentity(wsId: string): Promise<IdentityDTO> {
  return apiFetch<IdentityDTO>(identityBaseForWorkspace(wsId))
}
