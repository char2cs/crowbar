import { useEffect, useEffectEvent, useId } from 'react'
import { ArrowDownIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { hasNativeChrome, onNativeChromeIntent, setNativeJumpToLatest } from '@/lib/native-chrome'

const LABEL = 'Jump to latest'

interface JumpToLatestProps {
  onJump: () => void
  dockTop: number
}

const push = (owner: string, bottom: number | null) =>
  setNativeJumpToLatest({ owner, visible: bottom !== null, bottom: bottom ?? 0, label: LABEL })

/** The phone's chrome draws this button; the web says when it is up, how high, and never under the IDE. */
function NativeJumpToLatest({ onJump, dockTop }: JumpToLatestProps) {
  const onChat = useMobileScreenStore((s) => s.screen === 'chat')
  const bottom = onChat ? dockTop + 12 : null
  const owner = useId()
  useEffect(() => push(owner, bottom), [owner, bottom])
  useEffect(() => () => push(owner, null), [owner])
  const jump = useEffectEvent(onJump)
  useEffect(
    () =>
      onNativeChromeIntent((intent) => {
        if (intent.type === 'jumpToLatest' && intent.owner === owner) jump()
      }),
    [owner],
  )
  return null
}

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
