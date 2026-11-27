import { expect, vi } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'
import { toastManager } from '@/lib/toast-manager'
import { useConnectionStore } from '@/lib/ws/connection-store'

const MIGRATING = 'Crowbar is upgrading its data (step 2 of 7)'

export function watchToasts() {
  useConnectionStore.setState(useConnectionStore.getInitialState())
  return vi.spyOn(toastManager, 'add').mockClear()
}

export function refusal(message: string, code = 'conflict'): ApiError {
  return new ApiError(message, 409, code)
}

export const UNWORDED = { status: 0 }

export function migratingRefusal(): ApiError {
  return new ApiError(MIGRATING, 503, 'daemon_migrating')
}

export function expectReported(
  add: ReturnType<typeof watchToasts>,
  action: string,
  message: string,
) {
  expect(add).toHaveBeenCalledWith(
    expect.objectContaining({ type: 'error', title: action, description: message }),
  )
}

export function expectMigratingShown(add: ReturnType<typeof watchToasts>) {
  expect(useConnectionStore.getState().migrating).toEqual({ message: MIGRATING })
  expect(add).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }))
}
