import { Toast } from '@base-ui/react/toast'

type ToastManager = ReturnType<typeof Toast.createToastManager>
type ToastOptions = Parameters<ToastManager['add']>[0]

// base-ui's manager only emits: a toast added while nothing shows it (no viewport or shell
// forwarder yet: a boot failure) would be lost, so it waits for the first listener.
function heldUntilShown(manager: ToastManager): ToastManager {
  const held = new Map<string, ToastOptions>()
  const heldPromises: (() => void)[] = []
  let listening = 0
  return {
    ...manager,
    ' subscribe'(listener) {
      const stop = manager[' subscribe'](listener)
      listening++
      for (const options of held.values()) manager.add(options)
      held.clear()
      for (const replay of heldPromises.splice(0)) replay()
      return () => {
        listening--
        stop()
      }
    },
    add(options) {
      if (listening > 0) return manager.add(options)
      const id = options.id || crypto.randomUUID()
      held.set(id, { ...options, id })
      return id
    },
    close(id) {
      if (id === undefined) held.clear()
      else held.delete(id)
      manager.close(id)
    },
    promise(value, options) {
      if (listening > 0) return manager.promise(value, options)
      heldPromises.push(() => void manager.promise(value, options))
      return value
    },
    update(id, updates) {
      const options = held.get(id)
      if (options) held.set(id, { ...options, ...updates })
      manager.update(id, updates)
    },
  }
}

export const toastManager: ToastManager = heldUntilShown(Toast.createToastManager())
