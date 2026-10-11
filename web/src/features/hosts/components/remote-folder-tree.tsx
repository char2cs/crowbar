import { memo, useCallback, useId, useMemo } from 'react'
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
    case 'more-files':
      return `${note.hidden} more files not shown`
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
  treeRef: (tree: HTMLDivElement | null) => void
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

interface FolderTreeRowProps {
  row: FolderRow
  id: string
  hostName: string
  selected: boolean
  active: boolean
  onToggle: (path: string, expanded: boolean) => void
  /** The row was clicked: it becomes the active row, and a folder is picked. */
  onChoose: (key: string) => void
}

// Memoised, so a key press draws again only the two rows whose `active` it changed.
const FolderTreeRow = memo(function FolderTreeRow({
  row,
  id,
  hostName,
  selected,
  active,
  onToggle,
  onChoose,
}: FolderTreeRowProps) {
  if (row.kind === 'note') {
    return (
      <TreeItem
        id={id}
        level={row.level}
        inert
        active={active}
        icon={<NoteIcon note={row.note} />}
        className={row.note.type === 'failed' ? 'text-destructive' : 'text-muted-foreground'}
      >
        {noteText(row.note, hostName)}
      </TreeItem>
    )
  }
  return (
    <TreeItem
      id={id}
      level={row.level}
      expanded={row.expanded}
      inert={!row.entry.isDir}
      className={row.entry.isDir ? undefined : 'opacity-40'}
      selected={selected}
      active={active}
      icon={<EntryIcon entry={row.entry} expanded={row.expanded} />}
      onToggle={() => onToggle(row.key, !row.expanded)}
      onClick={() => onChoose(row.key)}
    >
      {row.entry.name}
    </TreeItem>
  )
})

/** The listed part of a host's filesystem: folders expand in place and can be picked;
 *  files are shown but inert. */
export function RemoteFolderTree({
  treeRef,
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

  const pick = useCallback(
    (key: string) => {
      const row = rows[indexOf.get(key) ?? -1]
      if (row?.kind === 'entry' && row.entry.isDir) onPickFolder(row.entry)
    },
    [rows, indexOf, onPickFolder],
  )
  const choose = useCallback(
    (key: string) => {
      onActiveKeyChange(key)
      pick(key)
    },
    [onActiveKeyChange, pick],
  )

  return (
    <Tree
      ref={treeRef}
      aria-label={`Folders on ${hostName}`}
      items={rows}
      activeKey={activeKey}
      onActiveKeyChange={onActiveKeyChange}
      onExpandedChange={onExpandedChange}
      onSelect={pick}
      itemId={(key) => `${baseId}-${indexOf.get(key)}`}
    >
      {rows.map((row, index) => (
        <FolderTreeRow
          key={row.key}
          row={row}
          id={`${baseId}-${index}`}
          hostName={hostName}
          selected={row.key === selectedPath}
          active={row.key === activeKey}
          onToggle={onExpandedChange}
          onChoose={choose}
        />
      ))}
    </Tree>
  )
}
