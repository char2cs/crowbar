import {
  useCallback,
  useEffect,
  useEffectEvent,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
} from 'react'
import type { ReactNode, Ref, RefObject } from 'react'
import { ComposerActionButton } from '@/features/agent/composer/composer-action-button'
import { IS_MOBILE } from '@/utils/platform'

const GHOST_PROMPTS = [
  ['GOAL', 'What is different when this is finished.'],
  ['WHERE', 'Files, screens or branches involved. Type @ to point at one.'],
  ['KEEP', 'What must not change, and how you will check the result.'],
] as const

function firstLineTop(headerClearancePx: number): number {
  return IS_MOBILE ? 30 + headerClearancePx + 38 : 48 + headerClearancePx + 40
}
const HANDLE_LEAD = 4

function lastLineTop(doc: HTMLElement, headerClearancePx = 0): number {
  const editable = doc.querySelector<HTMLElement>('[data-slate-editor]')
  const last = editable?.lastElementChild
  if (!last || !editable?.textContent) return firstLineTop(headerClearancePx)
  return last.getBoundingClientRect().bottom - doc.getBoundingClientRect().top
}

export interface AgentEmptyDocumentHandle {
  getHandleRect: () => DOMRect | null
}

export interface AgentEmptyDocumentProps {
  docRef: RefObject<HTMLDivElement | null>
  hasText: boolean
  onSubmit: () => void
  controls: ReactNode
  attach: ReactNode
  working?: boolean
  canStop?: boolean
  sending: boolean
  onStop?: () => void
  headerClearancePx?: number
  banner?: ReactNode
  ref?: Ref<AgentEmptyDocumentHandle>
}

export function AgentEmptyDocument({
  docRef,
  hasText,
  onSubmit,
  controls,
  attach,
  working = false,
  canStop = false,
  sending,
  onStop,
  headerClearancePx = 0,
  banner,
  ref,
}: AgentEmptyDocumentProps) {
  const handleRef = useRef<HTMLDivElement>(null)

  useImperativeHandle(
    ref,
    () => ({
      getHandleRect: () => handleRef.current?.getBoundingClientRect() ?? null,
    }),
    [],
  )

  const place = useCallback(() => {
    const doc = docRef.current
    const handle = handleRef.current
    if (!doc || !handle) return
    const top = lastLineTop(doc, headerClearancePx)
    handle.style.transform = `translateY(${Math.round(top + HANDLE_LEAD)}px)`
  }, [docRef, headerClearancePx])

  useLayoutEffect(place)

  const onSelectionChange = useEffectEvent(() => {
    const anchor = document.getSelection()?.anchorNode ?? null
    if (!anchor || !docRef.current?.contains(anchor)) return
    place()
  })
  useEffect(() => {
    document.addEventListener('selectionchange', onSelectionChange)
    return () => document.removeEventListener('selectionchange', onSelectionChange)
  }, [])

  return (
    <div ref={handleRef} className="dochandle" data-touch={IS_MOBILE || undefined}>
      {!hasText && (
        <dl className="ghosts">
          {GHOST_PROMPTS.map(([label, hint]) => (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{hint}</dd>
            </div>
          ))}
        </dl>
      )}
      <div className="inner">
        {banner ? (
          <div className="banner">{banner}</div>
        ) : (
          <div className="grp">
            <span className="side">{controls}</span>
            <span className="side">
              {attach}
              <ComposerActionButton
                hasText={hasText}
                working={working}
                canStop={canStop}
                sending={sending}
                onSend={onSubmit}
                onStop={onStop}
              />
            </span>
          </div>
        )}
      </div>
    </div>
  )
}
