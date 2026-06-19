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

interface CheckoutResult {
  success: boolean
  hasChanges: boolean
  message: string
}

export const getBranches = async (wsId: string): Promise<string[]> => {
  try {
    const branches = await apiFetch<BranchDTO[]>(
      `${workspaceBase(wsId)}/git/branches`,
    )
    return branches.map((b) => b.name)
  } catch {
    return []
  }
}

export const checkoutBranch = async (wsId: string, branchName: string): Promise<CheckoutResult> => {
  try {
    await apiFetch(`${workspaceBase(wsId)}/git/checkout`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ branch: branchName }),
    })
    return { success: true, hasChanges: false, message: '' }
  } catch (error) {
    return { success: false, hasChanges: false, message: String(error) }
  }
}

export const createBranch = async (
  wsId: string,
  branchName: string,
  fromBranch?: string,
): Promise<boolean> => {
  try {
    await apiFetch(`${workspaceBase(wsId)}/git/branches`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: branchName, ...(fromBranch && { source: fromBranch }) }),
    })
    return true
  } catch (error) {
    console.error('git create branch failed:', error)
    return false
  }
}

export const deleteBranch = async (wsId: string, branchName: string): Promise<boolean> => {
  try {
    await apiFetch(
      `${workspaceBase(wsId)}/git/branches/${encodeURIComponent(branchName)}`,
      { method: 'DELETE' },
    )
    return true
  } catch (error) {
    console.error('git delete branch failed:', error)
    return false
  }
}
