import { expect, vi } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'
import { toastManager } from '@/lib/toast-manager'
import { resetConnectionState, useConnectionStore } from '@/lib/ws/connection-store'

const MIGRATING = 'Crowbar is upgrading its data (step 2 of 7)'

/** Watches every toast, report()'s and toast-store's alike. */
export function watchToasts() {
  resetConnectionState()
  return vi.spyOn(toastManager, 'add').mockClear()
}

/** A refusal the daemon words itself, as apiFetch throws it. */
export function refusal(message: string, code = 'conflict'): ApiError {
  return new ApiError(message, 409, code)
}

/** The daemon refusing every call while it prepares its home. */
export function migratingRefusal(): ApiError {
  return new ApiError(MIGRATING, 503, 'daemon_migrating')
}

/** The failure read as report() words it: the action, then the daemon's message. */
export function expectReported(
  add: ReturnType<typeof watchToasts>,
  action: string,
  message: string,
) {
  expect(add).toHaveBeenCalledWith(
    expect.objectContaining({ type: 'error', title: action, description: message }),
  )
}

/** A migrating refusal goes to the migrating screen, never an error toast. */
export function expectMigratingShown(add: ReturnType<typeof watchToasts>) {
  expect(useConnectionStore.getState().migrating).toEqual({ message: MIGRATING })
  expect(add).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }))
}
