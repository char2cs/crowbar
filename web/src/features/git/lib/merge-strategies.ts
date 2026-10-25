import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'

export const MERGE_STRATEGIES: {
  value: MergeStrategy
  label: string
  confirm: string
  desc: string
}[] = [
  {
    value: 'merge',
    label: 'Create a merge commit',
    confirm: 'Create merge commit',
    desc: 'All commits, plus a merge commit',
  },
  {
    value: 'squash',
    label: 'Squash and merge',
    confirm: 'Squash & merge',
    desc: 'One combined commit',
  },
  {
    value: 'rebase',
    label: 'Rebase and merge',
    confirm: 'Rebase & merge',
    desc: 'Replay commits, no merge commit',
  },
]
