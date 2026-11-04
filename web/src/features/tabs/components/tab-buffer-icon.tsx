import {
  GitBranch,
  Globe,
  GitPullRequest,
  Package,
  Plus,
  TerminalWindow as Terminal,
} from '@phosphor-icons/react'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import type { PaneContent } from '@/features/panes/types/pane-content'

/** A tab's kind, as every tab strip marks it. */
export function TabBufferIcon({ buffer }: { buffer: PaneContent }) {
  if (buffer.path === 'extensions://marketplace')
    return <Package className="text-muted-foreground" />
  switch (buffer.type) {
    case 'branchReview':
      return <GitPullRequest className="text-muted-foreground" />
    case 'commitDiff':
      return <GitBranch className="text-muted-foreground" />
    case 'terminal':
      return <Terminal className="text-muted-foreground" />
    case 'browser':
      return <Globe className="text-muted-foreground" />
    case 'newTabPage':
      return <Plus className="text-muted-foreground" />
    default:
      return (
        <FileExplorerIcon
          fileName={buffer.name}
          isDir={false}
          className="text-muted-foreground"
          size={14}
        />
      )
  }
}
