import { vi, test, expect, beforeEach, afterEach } from 'vitest'
import { simulateStream } from '@/lib/mock/simulate-stream'

beforeEach(() => { vi.useFakeTimers() })
afterEach(() => { vi.useRealTimers() })

test('delivers all chunks then calls onDone', () => {
  const chunks: string[] = []
  const onDone = vi.fn()

  simulateStream('hello world foo', chunk => chunks.push(chunk), onDone)
  vi.runAllTimers()

  expect(chunks.join('')).toBe('hello world foo')
  expect(onDone).toHaveBeenCalledOnce()
})

test('cancel() stops further chunks and never calls onDone', () => {
  const chunks: string[] = []
  const onDone = vi.fn()

  const cancel = simulateStream('hello world foo', chunk => chunks.push(chunk), onDone)

  vi.advanceTimersByTime(400) // initial delay → fires 'hello'
  vi.advanceTimersByTime(40)  // first inter-word delay → fires 'world'
  cancel()

  vi.runAllTimers() // nothing more should fire

  expect(chunks.length).toBe(2) // 'hello' + ' world'
  expect(onDone).not.toHaveBeenCalled()
})

test('cancel() is idempotent — calling twice does not throw', () => {
  const cancel = simulateStream('hi', () => {}, () => {})
  expect(() => { cancel(); cancel() }).not.toThrow()
})
