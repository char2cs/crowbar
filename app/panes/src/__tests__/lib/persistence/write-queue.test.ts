import { describe, expect, it } from 'vitest'
import { WriteQueue } from '@/lib/persistence/write-queue'

function heldJob(ran: string[], outcome: 'resolves' | 'rejects') {
  let settle: (failure?: Error) => void = () => {}
  const done = new Promise<void>((resolve, reject) => {
    settle = (failure) => (failure ? reject(failure) : resolve())
  })
  const release = () => {
    ran.push('first')
    settle(outcome === 'rejects' ? new Error('aborted') : undefined)
  }
  return { run: () => done, release }
}

describe('WriteQueue', () => {
  it.each(['resolves', 'rejects'] as const)(
    'runs the next job once one that %s settles, and never fails its caller',
    async (outcome) => {
      const queue = new WriteQueue()
      const ran: string[] = []
      const first = heldJob(ran, outcome)
      const settled = [queue.push(first.run), queue.push(async () => ran.push('second'))]
      await Promise.resolve()
      expect(ran).toEqual([])

      first.release()

      await expect(Promise.all(settled)).resolves.toEqual([undefined, undefined])
      expect(ran).toEqual(['first', 'second'])
    },
  )
})
