import { PointerSensor } from '@dnd-kit/core'

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
