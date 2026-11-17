import { performImportBranches } from '@/components/sidebar/lib/import-branches'
import type { Repo } from '@/core/entities/repo-types'
import { RepoImportDialog } from './repo-import-dialog'
import { resolveRow } from './open-actions'

/** The import dialog for the project-home row `repoRowId` (null: closed), importing into its repo. */
export function RowBranchImportDialog({
  repoRowId,
  repos,
  onClose,
}: {
  repoRowId: string | null
  repos: readonly Repo[]
  onClose: () => void
}) {
  const repo = repoRowId != null ? resolveRow(repos, repoRowId)?.repo : undefined
  return (
    <RepoImportDialog
      projectId={repo?.projectId ?? ''}
      repoId={repo?.id ?? ''}
      defaultBranch={repo?.defaultBranch ?? ''}
      open={repoRowId != null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
      onImport={(branches, lockedBranches) => {
        if (repo) performImportBranches(repo.id, branches, lockedBranches)
      }}
    />
  )
}
