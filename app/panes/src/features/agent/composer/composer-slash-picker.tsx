import { Fragment, useEffect, useRef, type ReactNode } from 'react'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { commandGroup } from '@/features/agent/hooks/use-slash-catalog'
import { cn } from '@/lib/utils'

export interface PickerItem {
  id: string
  insertText: string
  description: string
  terminal?: boolean
  kind?: 'skill' | 'builtin' | 'command'
  name?: string
  dir?: string
  isDir?: boolean
}

interface ComposerSlashPickerProps {
  variant?: 'commands' | 'files'
  items: PickerItem[]
  selected: number
  query?: string
  scope?: string
  total?: number
  onSelect: (item: PickerItem) => void
}

const GROUPS = ['Commands', 'Runs in the terminal', 'Skills']

function marked(text: string, query: string, through = 0): ReactNode {
  const at = query ? text.toLowerCase().indexOf(query.toLowerCase()) : -1
  const edges =
    at >= 0 ? [0, through, at, at + query.length, text.length] : [0, through, text.length]
  const cuts = [...new Set(edges)].sort((a, b) => a - b)
  return cuts.slice(0, -1).map((start, i) => {
    const slice = text.slice(start, cuts[i + 1])
    const hit = at >= 0 && start >= at && start < at + query.length
    const node = hit ? <u>{slice}</u> : slice
    return start < through ? (
      // react-doctor-disable-next-line no-array-index-as-key -- slices of one fixed string, never reordered
      <span key={i} className="ns">
        {node}
      </span>
    ) : (
      // react-doctor-disable-next-line no-array-index-as-key -- slices of one fixed string, never reordered
      <Fragment key={i}>{node}</Fragment>
    )
  })
}

function CommandRow({ item, on, query }: { item: PickerItem; on: boolean; query: string }) {
  const text = item.insertText.trim()
  const through = item.kind === 'skill' && text.includes(':') ? text.indexOf(':') + 1 : 0
  return (
    <>
      <span className={cn('c', item.kind === 'skill' && 'wide')}>
        {marked(text, query.replace(/^\//, ''), through)}
      </span>
      {item.description && <span className="d">{item.description}</span>}
      {on && <span className="k">↵</span>}
    </>
  )
}

function FileRow({ item, query }: { item: PickerItem; query: string }) {
  const name = item.name ?? item.insertText
  return (
    <>
      <span className="fi">
        <FileExplorerIcon fileName={name} isDir={item.isDir} size={14} />
      </span>
      <span className="fn">{marked(item.isDir ? `${name}/` : name, query)}</span>
      <span className="fp">{item.dir}</span>
      {item.isDir && <span className="tag">Folder</span>}
    </>
  )
}

export function ComposerSlashPicker({
  variant = 'commands',
  items,
  selected,
  query = '',
  total,
  scope,
  onSelect,
}: ComposerSlashPickerProps) {
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>('[aria-selected="true"]')?.scrollIntoView({
      block: 'nearest',
    })
  }, [selected])

  if (items.length === 0) return null
  const files = variant === 'files'
  return (
    <div
      ref={listRef}
      className="slash"
      id="agent-skill-picker"
      role="listbox"
      aria-label={files ? 'Files' : 'Commands'}
    >
      {files && <div className="gh">{scope ? `Files in ${scope}` : 'Files'}</div>}
      <div className="list">
        {items.map((item, index) => {
          const group = files ? -1 : commandGroup({ kind: item.kind, terminal: item.terminal })
          const opens =
            !files &&
            (index === 0 ||
              group !==
                commandGroup({
                  kind: items[index - 1]!.kind,
                  terminal: items[index - 1]!.terminal,
                }))
          return (
            <Fragment key={item.id}>
              {opens && <div className="gh">{GROUPS[group]}</div>}
              <button
                type="button"
                role="option"
                aria-selected={index === selected}
                className={cn('row', index === selected && 'on', !files && 'two')}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => onSelect(item)}
              >
                {files ? (
                  <FileRow item={item} query={query} />
                ) : (
                  <CommandRow item={item} on={index === selected} query={query} />
                )}
              </button>
            </Fragment>
          )
        })}
      </div>
      <div className="pf">
        <span>
          {files
            ? `${items.length} ${items.length === 1 ? 'match' : 'matches'}`
            : `${items.length} of ${total ?? items.length} commands`}
        </span>
        <span className="grow" />
        <span>
          <kbd>↑</kbd>
          <kbd>↓</kbd>move
        </span>
        <span>
          <kbd>↵</kbd>
          {files ? 'mention' : 'run'}
        </span>
        {files ? (
          <span>
            <kbd>tab</kbd>open folder
          </span>
        ) : (
          <span>
            <kbd>esc</kbd>close
          </span>
        )}
      </div>
    </div>
  )
}
