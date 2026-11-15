import { describe, expect, it } from 'vitest'
import { PaneRequestError } from '@/sdk/pane-request-error'

describe('PaneRequestError', () => {
  it('carries the failure code and message', () => {
    const error = new PaneRequestError({ code: 'denied', message: 'no access' })
    expect(error).toBeInstanceOf(Error)
    expect(error).toMatchObject({ name: 'PaneRequestError', code: 'denied', message: 'no access' })
  })
})
