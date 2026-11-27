import { XIcon } from '@phosphor-icons/react'
import { percentOf, rowNote, type Staged } from '@/features/agent/composer/lib/attach-staged'

export function AttachFileList({
  items,
  onRemove,
}: {
  items: Staged[]
  onRemove: (id: number) => void
}) {
  if (items.length === 0) return null
  return (
    <div className="attach-files" data-testid="attach-file-list">
      {items.map((item) => {
        const percent = percentOf(item) ?? undefined
        return (
          <div key={item.id} className="attach-file" data-status={item.status}>
            <div className="attach-file-text">
              <div className="attach-file-name">{item.name}</div>
              <div className="attach-file-note">
                {rowNote(item)}
                {item.retry && (
                  <button type="button" className="attach-retry" onClick={item.retry}>
                    Retry
                  </button>
                )}
              </div>
              {item.status === 'uploading' && (
                <div
                  className="attach-bar"
                  role="progressbar"
                  aria-label={`Uploading ${item.name}`}
                  aria-valuenow={percent}
                  data-determinate={percent === undefined ? undefined : ''}
                >
                  <i style={percent === undefined ? undefined : { width: `${percent}%` }} />
                </div>
              )}
            </div>
            <button
              type="button"
              className="attach-x"
              aria-label={`Remove ${item.name}`}
              onClick={() => onRemove(item.id)}
            >
              <XIcon size={13} />
            </button>
          </div>
        )
      })}
    </div>
  )
}
