import { PointerSensor } from '@dnd-kit/core'

// Skips drag activation when the pointer event originates on a [data-no-dnd]
// element (e.g. the tab close button). Without this, DndKit captures the pointer
// on the close button's pointerdown, which fires handleDragStart → handleTabSelect
// before the close button's onClick can run, causing the tab to be activated
// instead of closed.
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
