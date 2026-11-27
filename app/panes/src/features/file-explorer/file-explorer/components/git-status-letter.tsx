import type { FileTreeGitStatusDecoration } from '@/features/file-explorer/lib/file-tree-git-status'
import { cn } from '@/utils/cn'

export function GitStatusLetter({ decoration }: { decoration: FileTreeGitStatusDecoration }) {
  return (
    <span
      className={cn(
        'file-tree-git-letter shrink-0 select-none font-mono text-[10px] leading-none opacity-80',
        decoration.colorClassName,
      )}
      aria-label={decoration.label}
    >
      {decoration.statusLetter}
    </span>
  )
}
