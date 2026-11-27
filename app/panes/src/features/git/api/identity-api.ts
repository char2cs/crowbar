import { apiFetch } from '@/lib/api/fetch'
import { identityBaseForWorkspace } from '@/lib/workspace-scope-url'

export interface IdentityDTO {
  login: string
  displayName: string
  avatarUrl: string
}

export async function getIdentity(wsId: string): Promise<IdentityDTO> {
  return apiFetch<IdentityDTO>(identityBaseForWorkspace(wsId))
}
