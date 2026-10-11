import { useId, useMemo } from 'react'
import { FileIcon, FolderIcon, FolderOpenIcon, WarningIcon } from '@phosphor-icons/react'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { Tree, TreeItem } from '@/components/ui/tree'
import type { BrowseEntry } from '@/features/hosts/lib/browse'
import {
  folderRows,
  type FolderNote,
  type FolderRow,
  type Listing,
} from '@/features/hosts/lib/folder-rows'

/** What a failed request says, for showing it. */
export function failureText(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function noteText(note: FolderNote, hostName: string): string {
  switch (note.type) {
    case 'waiting':
      return `Waiting for ${hostName} to answer`
    case 'loading':
      return 'Loading…'
    case 'empty':
      return 'Empty folder'
    case 'failed':
      return `Can't open: ${failureText(note.error)}`
    case 'more':
      return `${note.hidden} more not shown`
  }
}

function NoteIcon({ note }: { note: FolderNote }) {
  if (note.type === 'loading') return <LoadingSpinner compact label="Loading folder" />
  if (note.type === 'failed') return <WarningIcon aria-hidden="true" className="size-3.5" />
  return null
}

function EntryIcon({ entry, expanded }: { entry: BrowseEntry; expanded?: boolean }) {
  const Icon = !entry.isDir ? FileIcon : expanded ? FolderOpenIcon : FolderIcon
  return <Icon aria-hidden="true" className="size-3.5 shrink-0 text-muted-foreground" />
}

interface RemoteFolderTreeProps {
  hostName: string
  entries: readonly BrowseEntry[]
  listings: Readonly<Record<string, Listing>>
  expanded: ReadonlySet<string>
  selectedPath: string | null
  activeKey: string | null
  onActiveKeyChange: (key: string) => void
  onExpandedChange: (path: string, expanded: boolean) => void
  /** A folder was clicked or chosen from the keyboard. */
  onPickFolder: (entry: BrowseEntry) => void
}

/** The listed part of a host's filesystem: folders expand in place and can be picked;
 *  files are shown but inert. */
export function RemoteFolderTree({
  hostName,
  entries,
  listings,
  expanded,
  selectedPath,
  activeKey,
  onActiveKeyChange,
  onExpandedChange,
  onPickFolder,
}: RemoteFolderTreeProps) {
  const baseId = useId()
  const rows = useMemo(
    () => folderRows('/', entries, listings, expanded),
    [entries, listings, expanded],
  )
  const indexOf = useMemo(() => new Map(rows.map((row, i) => [row.key, i])), [rows])
  const byKey = (key: string): FolderRow | undefined => rows[indexOf.get(key) ?? -1]

  function pick(key: string) {
    const row = byKey(key)
    if (row?.kind === 'entry' && row.entry.isDir) onPickFolder(row.entry)
  }

  return (
    <Tree
      aria-label={`Folders on ${hostName}`}
      items={rows}
      activeKey={activeKey}
      onActiveKeyChange={onActiveKeyChange}
      onExpandedChange={onExpandedChange}
      onSelect={pick}
      itemId={(key) => `${baseId}-${indexOf.get(key)}`}
    >
      {rows.map((row, index) =>
        row.kind === 'note' ? (
          <TreeItem
            key={row.key}
            id={`${baseId}-${index}`}
            level={row.level}
            inert
            active={row.key === activeKey}
            icon={<NoteIcon note={row.note} />}
            className={row.note.type === 'failed' ? 'text-destructive' : undefined}
          >
            {noteText(row.note, hostName)}
          </TreeItem>
        ) : (
          <TreeItem
            key={row.key}
            id={`${baseId}-${index}`}
            level={row.level}
            expanded={row.expanded}
            inert={!row.entry.isDir}
            selected={row.key === selectedPath}
            active={row.key === activeKey}
            icon={<EntryIcon entry={row.entry} expanded={row.expanded} />}
            onToggle={() => onExpandedChange(row.key, !row.expanded)}
            onClick={() => {
              onActiveKeyChange(row.key)
              pick(row.key)
            }}
          >
            {row.entry.name}
          </TreeItem>
        ),
      )}
    </Tree>
  )
}
