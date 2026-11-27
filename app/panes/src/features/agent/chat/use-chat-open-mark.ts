import { useEffect } from 'react'
import { markEnd, markStart } from '@/lib/perf/instrumentation'

export function useChatOpenMark(opened: boolean) {
  useEffect(() => markStart('chat.open'), [])
  useEffect(() => {
    if (!opened) return
    const raf = requestAnimationFrame(() => {
      markEnd('chat.open')
    })
    return () => cancelAnimationFrame(raf)
  }, [opened])
}
