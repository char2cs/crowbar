import type { GitCommit, GitDiff, GitStatus } from '../types/git-types'
import { reviewFilesSummaryToChangedFiles } from '../utils/review-file-summary-to-git-diff'
import { getGitLog } from './git-commits-api'
import { getGitStatus } from './git-status-api'
import { getReviewFiles } from './review-api'

export const COMMITS_PER_PAGE = 50

/** The sidebar's changed-files list; null when the summary could not be fetched. */
export async function fetchReviewFiles(wsId: string): Promise<GitDiff[] | null> {
  try {
    return reviewFilesSummaryToChangedFiles(await getReviewFiles({ wsId }))
  } catch {
    return null
  }
}

export interface GitData {
  status: GitStatus | null
  commits: GitCommit[]
  reviewFiles: GitDiff[] | null
}

export async function fetchGitData(wsId: string): Promise<GitData> {
  const [status, commits, reviewFiles] = await Promise.all([
    getGitStatus(wsId),
    getGitLog(wsId, COMMITS_PER_PAGE, 0),
    fetchReviewFiles(wsId),
  ])
  return { status, commits, reviewFiles }
}
