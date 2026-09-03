import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  canPersistPromptQueue,
  clearPersistedPromptQueue,
  isPromptTextWithinLimit,
  loadPromptQueue,
  MAX_PROMPT_QUEUE_ITEMS,
  MAX_PROMPT_QUEUE_VALUE_BYTES,
  MAX_PROMPT_QUEUES_TOTAL_BYTES,
  MAX_PROMPT_TEXT_BYTES,
  promptQueueStorageKey,
  savePromptQueue,
  type PromptQueueItem,
} from '@/features/agent/lib/prompt-queue-persistence'

const item = (state: PromptQueueItem['state'] = 'queued'): PromptQueueItem => ({
  clientRequestId: 'request-1',
  text: 'Do the work',
  state,
  createdAt: '2026-08-16T00:00:00.000Z',
  baselineSequence: 7,
})

describe('prompt queue persistence', () => {
  beforeEach(() => localStorage.clear())

  it('round-trips a valid queue in a versioned workspace/chat key', () => {
    expect(savePromptQueue('w/1', 'c:1', [item()])).toBe(true)
    expect(promptQueueStorageKey('w/1', 'c:1')).toContain('w%2F1:c%3A1')
    expect(loadPromptQueue('w/1', 'c:1')).toEqual([item()])
  })

  it('promotes an interrupted in-flight request to outcome-uncertain without changing its id', () => {
    savePromptQueue('w1', 'c1', [item('submitting')])
    const [restored] = loadPromptQueue('w1', 'c1')
    expect(restored).toMatchObject({
      clientRequestId: 'request-1',
      state: 'outcome_uncertain',
    })
    expect(restored?.error).toMatch(/closed while.*submitted/i)
  })

  it('rejects and removes corrupt, oversized, or unversioned storage', () => {
    const key = promptQueueStorageKey('w1', 'c1')
    for (const bad of [
      '{broken',
      JSON.stringify({ version: 2, items: [item()] }),
      JSON.stringify({ version: 1, items: [{ ...item(), state: 'invented' }] }),
      JSON.stringify({
        version: 1,
        items: Array.from({ length: MAX_PROMPT_QUEUE_ITEMS + 1 }, () => item()),
      }),
    ]) {
      localStorage.setItem(key, bad)
      expect(loadPromptQueue('w1', 'c1')).toEqual([])
      expect(localStorage.getItem(key)).toBeNull()
    }
  })

  it('bounds prompt text by UTF-8 bytes, not JavaScript code units', () => {
    expect(isPromptTextWithinLimit('')).toBe(false)
    expect(isPromptTextWithinLimit('ok')).toBe(true)
    // Each emoji is four UTF-8 bytes.
    expect(isPromptTextWithinLimit('😀'.repeat(MAX_PROMPT_TEXT_BYTES / 4))).toBe(true)
    expect(isPromptTextWithinLimit(`${'😀'.repeat(MAX_PROMPT_TEXT_BYTES / 4)}a`)).toBe(false)
  })

  it('reports storage quota failure while leaving the caller in control of memory state', () => {
    const spy = vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
      throw new DOMException('quota', 'QuotaExceededError')
    })
    expect(savePromptQueue('w1', 'c1', [item()])).toBe(false)
    spy.mockRestore()
  })

  it('rejects a per-chat queue before it can consume the origin quota', () => {
    const maxText = 'x'.repeat(MAX_PROMPT_TEXT_BYTES)
    const oversized = Array.from({ length: 3 }, (_, index) => ({
      ...item(),
      clientRequestId: `request-${index}`,
      text: maxText,
    }))
    expect(canPersistPromptQueue('w1', 'large', oversized)).toBe(false)
    expect(savePromptQueue('w1', 'large', oversized)).toBe(false)
    expect(localStorage.getItem(promptQueueStorageKey('w1', 'large'))).toBeNull()
    expect(MAX_PROMPT_QUEUE_VALUE_BYTES).toBeLessThan(MAX_PROMPT_QUEUES_TOTAL_BYTES)
  })

  it('enforces one aggregate budget across chat queue keys', () => {
    const maxItem = { ...item(), text: 'x'.repeat(MAX_PROMPT_TEXT_BYTES) }
    let saved = 0
    for (let index = 0; index < 20; index++) {
      if (!savePromptQueue('w1', `chat-${index}`, [{ ...maxItem, clientRequestId: `r-${index}` }]))
        break
      saved++
    }
    expect(saved).toBeGreaterThan(0)
    expect(saved).toBeLessThan(20)
    expect(canPersistPromptQueue('w1', 'over-budget', [maxItem])).toBe(false)
  })

  it('clears only the deleted chat queue', () => {
    savePromptQueue('w1', 'c1', [item()])
    savePromptQueue('w1', 'c2', [{ ...item(), clientRequestId: 'request-2' }])
    clearPersistedPromptQueue('w1', 'c1')
    expect(loadPromptQueue('w1', 'c1')).toEqual([])
    expect(loadPromptQueue('w1', 'c2')).toHaveLength(1)
  })
})
