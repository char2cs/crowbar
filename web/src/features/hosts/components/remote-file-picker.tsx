import { useCallback, useRef, useState, type RefObject } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogPopup,
  DialogTitle,
} from '@/components/ui/dialog'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import {
  HostStatusDot,
  hostDotLabel,
  hostIsUnreachable,
} from '@/features/hosts/components/host-status-dot'
import { RemoteFolderTree } from '@/features/hosts/components/remote-folder-tree'
import { ROOT_PATH, useFolderListings } from '@/features/hosts/hooks/use-folder-listings'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import type { BrowseEntry } from '@/features/hosts/lib/browse'
import { childrenOf, failureText } from '@/features/hosts/lib/folder-rows'
import type { HostDotState } from '@/features/hosts/types'

interface RemoteFilePickerProps {
  /** The paired host whose filesystem is browsed — only ever that host. */
  hostId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The absolute path of the folder chosen, on `hostId`. */
  onSelect: (path: string) => void
}

/** Picks a folder on a remote host's own filesystem, where no native dialog can reach. */
export function RemoteFilePicker({ hostId, open, onOpenChange, onSelect }: RemoteFilePickerProps) {
  const listRef = useRef<HTMLDivElement>(null)
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPopup className="max-w-sm" initialFocus={() => listRef.current ?? true}>
        {/* Keyed: nothing listed on one host is ever shown, or applied, for another. */}
        <FolderBrowser
          key={hostId}
          hostId={hostId}
          listRef={listRef}
          onCancel={() => onOpenChange(false)}
          onChoose={(path) => {
            onSelect(path)
            onOpenChange(false)
          }}
        />
      </DialogPopup>
    </Dialog>
  )
}

function bannerText(name: string, state: HostDotState): string {
  if (state === 'needs-repair') return `${name} refuses this app until it is paired again.`
  return `${name} is not answering. Its folders load again once it does.`
}

interface FolderBrowserProps {
  hostId: string
  /** The folder list, which holds focus until the tree inside it can take it. */
  listRef: RefObject<HTMLDivElement | null>
  onCancel: () => void
  onChoose: (path: string) => void
}

function FolderBrowser({ hostId, listRef, onCancel, onChoose }: FolderBrowserProps) {
  const { name, state } = useHostView(hostId)
  const canAsk = !hostIsUnreachable(state)
  const folders = useFolderListings(hostId, canAsk)
  const [selected, setSelected] = useState<BrowseEntry | null>(null)
  const [activeKey, setActiveKey] = useState<string | null>(null)

  const treeRef = useRef<HTMLDivElement | null>(null)
  // Focus given to the list passes to the tree, whichever of the two arrives first.
  const takeFocus = useCallback(
    (tree: HTMLDivElement | null) => {
      treeRef.current = tree
      if (tree && document.activeElement === listRef.current) tree.focus()
    },
    [listRef],
  )

  const { expand } = folders
  const pickFolder = useCallback(
    (entry: BrowseEntry) => {
      setSelected(entry)
      expand(entry.path)
    },
    [expand],
  )

  return (
    <>
      <DialogHeader>
        <DialogTitle>Choose a folder</DialogTitle>
        <DialogDescription>
          {name === undefined
            ? 'This host is no longer paired, so its folders cannot be listed.'
            : `Browsing ${name}'s filesystem`}
        </DialogDescription>
      </DialogHeader>
      {name !== undefined && !canAsk && (
        <p role="status" className="mx-6 mb-3 rounded-md bg-warning/10 px-3 py-2 text-sm">
          {bannerText(name, state)}
        </p>
      )}
      {name !== undefined && (
        <div
          ref={listRef}
          role="group"
          aria-label="Folder list"
          tabIndex={-1}
          className="mx-3 mb-3 max-h-60 min-h-32 overflow-auto outline-none"
          onFocus={(event) => {
            if (event.target === event.currentTarget) treeRef.current?.focus()
          }}
        >
          <FolderRoot
            treeRef={takeFocus}
            hostName={name}
            canAsk={canAsk}
            folders={folders}
            selectedPath={selected?.path ?? null}
            activeKey={activeKey}
            onActiveKeyChange={setActiveKey}
            onPickFolder={pickFolder}
          />
        </div>
      )}
      <DialogFooter className="items-center sm:justify-between">
        <span className="flex items-center gap-1.5 text-muted-foreground text-xs">
          <HostStatusDot hostId={hostId} state={state} />
          {name ?? hostDotLabel(state)}
        </span>
        <span className="flex gap-2">
          <Button variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
          <Button
            disabled={selected === null || !canAsk}
            onClick={() => selected && onChoose(selected.path)}
          >
            {selected ? `Select "${selected.name}"` : 'Select'}
          </Button>
        </span>
      </DialogFooter>
    </>
  )
}

interface FolderRootProps {
  treeRef: (tree: HTMLDivElement | null) => void
  hostName: string
  canAsk: boolean
  folders: ReturnType<typeof useFolderListings>
  selectedPath: string | null
  activeKey: string | null
  onActiveKeyChange: (key: string) => void
  onPickFolder: (entry: BrowseEntry) => void
}

/** The top of the tree, or what stands in for it until it is listed. */
function FolderRoot({ hostName, canAsk, folders, ...tree }: FolderRootProps) {
  const { expand, collapse } = folders
  const setOpen = useCallback(
    (path: string, open: boolean) => (open ? expand(path) : collapse(path)),
    [expand, collapse],
  )
  const root = folders.listings[ROOT_PATH]
  const notice = 'flex min-h-32 flex-col items-center justify-center gap-2 text-center text-sm'
  if (root === undefined && !canAsk) {
    return <p className={notice}>Folders load once {hostName} answers.</p>
  }
  // Not listed yet while the host can be asked: the first request is on its way.
  if (root === undefined || root.status === 'loading') {
    return (
      <div className={notice}>
        <LoadingSpinner showLabel label="Loading folders…" />
      </div>
    )
  }
  if (root.status === 'failed') {
    return (
      <div role="alert" className={notice}>
        <p>
          Couldn't list folders on {hostName}: {failureText(root.error)}
        </p>
        <Button
          variant="outline"
          size="sm"
          disabled={!canAsk}
          onClick={() => folders.retry(ROOT_PATH)}
        >
          Retry
        </Button>
      </div>
    )
  }
  if (childrenOf(ROOT_PATH, root.entries).length === 0) {
    return <p className={notice}>This folder is empty.</p>
  }
  return (
    <RemoteFolderTree
      hostName={hostName}
      entries={root.entries}
      listings={folders.listings}
      expanded={folders.expanded}
      onExpandedChange={setOpen}
      {...tree}
    />
  )
}
