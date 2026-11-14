import { PointerSensor } from '@dnd-kit/core'

// Skips drag activation from a [data-no-dnd] element (e.g. a tab's close button); otherwise the
// drag start selects the tab before the close button's onClick runs.
export class NoDndPointerSensor extends PointerSensor {
  static activators = [
    {
      eventName: 'onPointerDown' as const,
      handler: ({ nativeEvent: event }: { nativeEvent: PointerEvent }): boolean => {
        if (!event.isPrimary || event.button !== 0) return false
        const target = event.target as HTMLElement | null
        if (target?.closest('[data-no-dnd]')) return false
        return true
      },
    },
  ]
}
