import { describe, expect, it } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'
import { problemOf } from '@/lib/errors/problem'

describe('problemOf', () => {
  it('reads the code and message an ApiError carries', () => {
    expect(problemOf(new ApiError('held', 409, 'chat_busy'))).toEqual({
      code: 'chat_busy',
      message: 'held',
    })
  })

  it('an ApiError without a code takes the category of its status', () => {
    expect(problemOf(new ApiError('gone', 404))).toEqual({ code: 'not_found', message: 'gone' })
    expect(problemOf(new ApiError('odd', 418))).toEqual({ code: 'internal', message: 'odd' })
  })

  it('an Error without a code is internal and keeps its message', () => {
    expect(problemOf(new Error('disk gone'))).toEqual({ code: 'internal', message: 'disk gone' })
  })

  it('a thrown string becomes an internal problem with that text', () => {
    expect(problemOf('nope')).toEqual({ code: 'internal', message: 'nope' })
  })
})
