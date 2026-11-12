import { useEffect, useEffectEvent } from 'react'
import { ArrowDownIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { hasNativeChrome, onNativeChromeIntent, setNativeJumpToLatest } from '@/lib/native-chrome'

const LABEL = 'Jump to latest'
/** The gap above the dock's top edge, as `.jump-latest` leaves it on the web. */
const GAP = 12

interface JumpToLatestProps {
  onJump: () => void
  /** Where the dock's top edge sits above the viewport's bottom. */
  dockTop: number
}

/** The phone's native chrome draws this button; the web only says when it is up and how high. */
function NativeJumpToLatest({ onJump, dockTop }: JumpToLatestProps) {
  const bottom = dockTop + GAP
  useEffect(() => {
    setNativeJumpToLatest({ visible: true, bottom, label: LABEL })
  }, [bottom])
  useEffect(() => () => setNativeJumpToLatest({ visible: false, bottom: 0, label: LABEL }), [])
  const jump = useEffectEvent(onJump)
  useEffect(
    () =>
      onNativeChromeIntent((intent) => {
        if (intent.type === 'jumpToLatest') jump()
      }),
    [],
  )
  return null
}

/** Back to the newest rows, for a reader up the page while the chat goes on below. */
export function JumpToLatest(props: JumpToLatestProps) {
  if (hasNativeChrome()) return <NativeJumpToLatest {...props} />
  return (
    <Button
      size="icon-sm"
      variant="secondary"
      className="jump-latest"
      aria-label={LABEL}
      onClick={props.onJump}
    >
      <ArrowDownIcon />
    </Button>
  )
}
