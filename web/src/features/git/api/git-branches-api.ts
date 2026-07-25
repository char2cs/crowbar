import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'

interface BranchDTO {
  name: string
  isCurrent: boolean
  isRemote: boolean
  ahead?: number
  behind?: number
  lastCommitDate?: string
}

export const getBranches = async (wsId: string): Promise<string[]> => {
  try {
    const branches = await apiFetch<BranchDTO[]>(`${workspaceBase(wsId)}/git/branches`)
    return branches.map((b) => b.name)
  } catch {
    return []
  }
}
