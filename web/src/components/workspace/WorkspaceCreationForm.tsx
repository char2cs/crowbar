import { useState } from 'react'
import { Button } from '@/components/ui/button'

interface Props {
  repos: { id: string; name: string }[]
  onSubmit: (data: { repoId: string; branch: string }) => void
  loading?: boolean
}

export function WorkspaceCreationForm({ repos, onSubmit, loading }: Props) {
  const [repoId, setRepoId] = useState(repos[0]?.id ?? '')
  const [branch, setBranch] = useState('')

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit({ repoId, branch })
      }}
    >
      <label className="flex flex-col gap-1 text-sm">
        <span className="text-muted-foreground">Repo</span>
        <select
          aria-label="Repo"
          value={repoId}
          onChange={(e) => setRepoId(e.target.value)}
          className="rounded-md border border-border bg-background px-3 py-2 text-foreground"
        >
          {repos.map((r) => (
            <option key={r.id} value={r.id}>
              {r.name}
            </option>
          ))}
        </select>
      </label>

      <label className="flex flex-col gap-1 text-sm">
        <span className="text-muted-foreground">Branch</span>
        <input
          aria-label="Branch"
          type="text"
          value={branch}
          onChange={(e) => setBranch(e.target.value)}
          placeholder="feature/my-feature"
          className="rounded-md border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground"
        />
      </label>

      <Button type="submit" disabled={!branch.trim() || loading}>
        {loading ? 'Creating…' : 'Create workspace'}
      </Button>
    </form>
  )
}
