import { beforeEach, describe, expect, it } from 'vitest'
import { entityStore } from '@/core/entities/entity-store'
import { selectReviewThreads } from '@/core/entities/select'
import type { ThreadDTO } from '@/lib/types'

const thread = (id: string, workspaceId: string) => ({ id, workspaceId }) as ThreadDTO

beforeEach(() => {
  entityStore.setState({
    byKind: {
      operation: {},
      review_thread: {
        a: { v: 1, data: thread('a', 'w1') },
        b: { v: 1, data: thread('b', 'w2') },
        c: { v: 2, data: thread('c', 'w1') },
      },
    },
  })
})

describe('selectReviewThreads', () => {
  it('selectReviewThreads returns the workspace threads and keeps identity when nothing changed', () => {
    const first = selectReviewThreads(entityStore.getState(), 'w1')
    expect(first.map((t) => t.id)).toEqual(['a', 'c'])

    entityStore.setState({ epoch: 'other' })
    expect(selectReviewThreads(entityStore.getState(), 'w1')).toBe(first)

    const { byKind } = entityStore.getState()
    entityStore.setState({
      byKind: {
        ...byKind,
        review_thread: { ...byKind.review_thread, d: { v: 3, data: thread('d', 'w1') } },
      },
    })
    expect(selectReviewThreads(entityStore.getState(), 'w1').map((t) => t.id)).toEqual([
      'a',
      'c',
      'd',
    ])
  })
})
