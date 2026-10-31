import { act } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest'
import { delta, ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'
import * as commands from '@/features/agent/api/agent-chat-commands'
import * as feeds from '@/features/agent/stores/chat-feed'
import {
  discardUnsent,
  holdForTerminal,
  retractNewest,
  retryPrompt,
  sendPrompt,
} from '@/features/agent/stores/chat-outbox'
import { applyFeedFrame, chatStore } from '@/features/agent/stores/chat-store'
import { ApiError } from '@/lib/errors/api-error'
import * as errors from '@/lib/errors/report'
import { toastManager } from '@/lib/toast-manager'

let submit: MockInstance<typeof commands.submitAgentPrompt>
let retract: MockInstance<typeof commands.retractPrompt>
let stop: MockInstance<typeof commands.stopChat>
let attach: MockInstance<typeof feeds.attachFeed>
let detach: ReturnType<typeof vi.fn<() => void>>
let reported: MockInstance<typeof errors.report>
let toasts: MockInstance<typeof toastManager.add>
let closed: MockInstance<typeof toastManager.close>

const accepted = { runnerId: 'r1', terminalSessionId: 'pty1' }

beforeEach(() => {
  chatStore.setState({ feeds: {}, outbox: {}, chats: {} })
  submit = vi.spyOn(commands, 'submitAgentPrompt').mockResolvedValue(accepted)
  retract = vi.spyOn(commands, 'retractPrompt')
  stop = vi.spyOn(commands, 'stopChat').mockResolvedValue()
  detach = vi.fn<() => void>()
  attach = vi.spyOn(feeds, 'attachFeed').mockReturnValue(detach)
  reported = vi.spyOn(errors, 'report').mockImplementation(() => {})
  toasts = vi.spyOn(toastManager, 'add').mockReturnValue('')
  closed = vi.spyOn(toastManager, 'close').mockImplementation(() => {})
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

const outbox = () => chatStore.getState().outbox.c1 ?? []
const send = (text: string, steer = false) => sendPrompt('w1', 'c1', { text, steer })
const sentId = (call = 0) => String(submit.mock.calls[call]?.[3])
const flush = () => act(async () => {})

function busy() {
  const l = ledger('c1')
  seedFeed('c1', [l.prompt('r0', 'earlier'), l.start('turn-0', 'r0')])
  return l
}

function deferred<T>() {
  let resolve: (value: T) => void = () => {}
  let reject: (error: unknown) => void = () => {}
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

describe('chat outbox', () => {
  it('symptom 5: Esc on a queued prompt restores it without a request', async () => {
    busy()
    send('wait your turn')
    expect(outbox()[0]?.state).toBe('queued')

    await expect(retractNewest('w1', 'c1')).resolves.toBe('wait your turn')

    expect(outbox()).toEqual([])
    expect(submit).not.toHaveBeenCalled()
    expect(retract).not.toHaveBeenCalled()
  })

  it('symptom 5: Esc before delivery retracts and restores the draft', async () => {
    const l = ledger('c1')
    seedFeed('c1', [])
    send('take this back')
    await flush()
    seedFeed('c1', [l.prompt(sentId(), 'take this back')])
    expect(outbox()).toEqual([])
    retract.mockResolvedValue({ state: 'retracted', text: 'take this back' })

    await expect(retractNewest('w1', 'c1')).resolves.toBe('take this back')

    expect(retract).toHaveBeenCalledWith('w1', 'c1', sentId())
    seedFeed('c1', [l.settle(sentId(), false, 'retracted')])
    expect(chatStore.getState().feeds.c1?.order).toEqual([])
  })

  it('symptom 5: Esc while the runner is spawning retracts and restores the draft', async () => {
    const l = ledger('c1')
    seedFeed('c1', [])
    send('first words')
    await flush()
    seedFeed('c1', [
      l.row('user_message', {
        requestId: sentId(),
        text: 'first words',
        origin: 'crowbar',
        delivery: 'restart',
        runnerId: 'r-spawning',
      }),
    ])
    retract.mockResolvedValue({ state: 'retracted', text: 'first words' })

    await expect(retractNewest('w1', 'c1')).resolves.toBe('first words')
    expect(retract).toHaveBeenCalledWith('w1', 'c1', sentId())
  })

  it('symptom 5: Esc after delivery with no output interrupts and restores', async () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', 'stop that'), l.start('turn-1', 'r1')])
    retract.mockResolvedValue({ state: 'interrupted', text: 'stop that' })

    await expect(retractNewest('w1', 'c1')).resolves.toBe('stop that')
    expect(retract).toHaveBeenCalledWith('w1', 'c1', 'r1')
    expect(stop).not.toHaveBeenCalled()
  })

  it('stops without restoring once the turn has output', async () => {
    const l = ledger('c1')
    seedFeed('c1', [
      l.prompt('r1', 'go'),
      l.start('turn-1', 'r1'),
      l.text('turn-1', 'm1', 0, 'Working'),
    ])

    await expect(retractNewest('w1', 'c1')).resolves.toBeNull()
    expect(stop).toHaveBeenCalledWith('w1', 'c1')
    expect(retract).not.toHaveBeenCalled()
  })

  it('keeps the text off the composer when the interrupted turn spoke before the answer', async () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', 'stop that'), l.start('turn-1', 'r1')])
    retract.mockImplementation(async () => {
      seedFeed('c1', [l.text('turn-1', 'm1', 0, 'Already started')])
      return { state: 'interrupted', text: 'stop that' }
    })

    await expect(retractNewest('w1', 'c1')).resolves.toBeNull()
  })

  it('has nothing to act on in an idle chat', () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', 'go'), l.start('turn-1', 'r1'), l.end('turn-1')])
    expect(retractNewest('w1', 'c1')).toBeNull()
  })

  it('retracts after the send resolves when Esc came first', async () => {
    seedFeed('c1', [])
    const sent = deferred<typeof accepted>()
    submit.mockReturnValue(sent.promise)
    send('too fast')
    retract.mockResolvedValue({ state: 'retracted', text: 'too fast' })

    const restored = retractNewest('w1', 'c1')
    expect(outbox()[0]).toMatchObject({ state: 'sending', retractOnAck: true })
    expect(retract).not.toHaveBeenCalled()
    sent.resolve(accepted)

    await expect(restored).resolves.toBe('too fast')
    expect(retract).toHaveBeenCalledWith('w1', 'c1', sentId())
    expect(outbox()).toEqual([])
  })

  it('keeps the text off the composer when a send Esc raced spoke before the retract', async () => {
    seedFeed('c1', [])
    const sent = deferred<typeof accepted>()
    submit.mockReturnValue(sent.promise)
    send('said twice')
    const l = ledger('c1')
    retract.mockImplementation(async () => {
      seedFeed('c1', [
        l.prompt(sentId(), 'said twice'),
        l.start('turn-1', sentId()),
        l.text('turn-1', 'm1', 0, 'Already started'),
        l.row('turn_ended', { status: 'interrupted', reason: 'retracted' }, { turnId: 'turn-1' }),
      ])
      return { state: 'interrupted', text: 'said twice' }
    })

    const restored = retractNewest('w1', 'c1')
    sent.resolve(accepted)

    await expect(restored).resolves.toBeNull()
    expect(chatStore.getState().feeds.c1?.order[0]).toBe(`p:${sentId()}`)
  })

  it('keeps the text off the composer when a send Esc raced was streaming its reply', async () => {
    seedFeed('c1', [])
    const sent = deferred<typeof accepted>()
    submit.mockReturnValue(sent.promise)
    send('streamed twice')
    const l = ledger('c1')
    retract.mockImplementation(async () => {
      seedFeed('c1', [l.prompt(sentId(), 'streamed twice'), l.start('turn-1', sentId())])
      applyFeedFrame('c1', {
        type: 'delta',
        delta: delta('m1', 1, 'Already', { turnId: 'turn-1' }),
      })
      seedFeed('c1', [
        l.row('turn_ended', { status: 'interrupted', reason: 'retracted' }, { turnId: 'turn-1' }),
      ])
      return { state: 'interrupted', text: 'streamed twice' }
    })

    const restored = retractNewest('w1', 'c1')
    sent.resolve(accepted)

    await expect(restored).resolves.toBeNull()
    expect(chatStore.getState().feeds.c1?.order).toEqual([`p:${sentId()}`, 'm:turn-1:m1', 'k:3'])
  })

  it('gives the text back when a send Esc raced was refused', async () => {
    seedFeed('c1', [])
    const sent = deferred<typeof accepted>()
    submit.mockReturnValue(sent.promise)
    send('lost the race')
    retract.mockRejectedValue(new ApiError('unknown prompt: not found', 404))

    const restored = retractNewest('w1', 'c1')
    sent.reject(new ApiError('busy: conflict', 409, 'chat_busy'))

    await expect(restored).resolves.toBe('lost the race')
    expect(outbox()).toEqual([])
    expect(submit).toHaveBeenCalledTimes(1)
  })

  it('restores the text when the retract answers retracted even though turn_started arrived', async () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', 'raced'), l.start('turn-1', 'r1')])
    retract.mockResolvedValue({ state: 'retracted', text: 'raced' })

    await expect(retractNewest('w1', 'c1')).resolves.toBe('raced')
  })

  it('keeps the feed attached while the outbox is non-empty', () => {
    busy()
    send('one')
    send('two')
    expect(attach).toHaveBeenCalledTimes(1)
    expect(attach).toHaveBeenCalledWith('c1')
    expect(detach).not.toHaveBeenCalled()

    discardUnsent('c1')
    expect(detach).toHaveBeenCalledTimes(1)
  })

  it('sends the next prompt on the turn_ended row, never on a clock', async () => {
    vi.useFakeTimers()
    const l = busy()
    send('after this turn')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(submit).not.toHaveBeenCalled()

    seedFeed('c1', [l.end('turn-0')])
    expect(submit).toHaveBeenCalledTimes(1)
    expect(submit.mock.calls[0]?.slice(0, 3)).toEqual(['w1', 'c1', 'after this turn'])
  })

  it('holds the outbox while a reset ledger replays', () => {
    const l = ledger('c1')
    seedFeed('c1', [l.start('turn-0'), l.end('turn-0')])
    act(() => applyFeedFrame('c1', { type: 'resync', seq: 0 }))
    send('after the replay')
    expect(submit).not.toHaveBeenCalled()

    act(() => applyFeedFrame('c1', { type: 'ready', epoch: 'test' }))
    expect(submit).toHaveBeenCalledTimes(1)
  })

  it('sends a prompt the daemon called busy again on the next row, under the same id', async () => {
    const l = ledger('c1')
    seedFeed('c1', [])
    submit.mockRejectedValueOnce(new ApiError('busy: conflict', 409, 'chat_busy'))
    send('patience')
    await flush()
    expect(outbox()[0]).toMatchObject({ state: 'queued' })
    expect(submit).toHaveBeenCalledTimes(1)

    seedFeed('c1', [l.start('turn-x'), l.end('turn-x')])
    expect(submit).toHaveBeenCalledTimes(2)
    expect(sentId(1)).toBe(sentId(0))
    expect(reported).not.toHaveBeenCalled()
  })

  it('reports a send refused before its row, but not a lost session', async () => {
    seedFeed('c1', [])
    submit.mockRejectedValueOnce(new ApiError('model is unknown: invalid argument', 400))
    send('one')
    await flush()
    expect(reported).toHaveBeenCalledTimes(1)
    expect(outbox()[0]).toMatchObject({
      state: 'failed',
      error: 'model is unknown: invalid argument',
    })

    discardUnsent('c1')
    submit.mockRejectedValueOnce(new ApiError('session lost: conflict', 409, 'session_lost'))
    send('two')
    await flush()
    expect(reported).toHaveBeenCalledTimes(1)
    expect(outbox()[0]?.error).toMatch(/session was lost/i)
  })

  it('sends the prompts queued behind one the daemon refused', async () => {
    busy()
    submit.mockRejectedValueOnce(new ApiError('no provider: unprocessable entity', 422))
    send('one')
    send('two')
    const l = ledger('c1', 3)
    seedFeed('c1', [l.end('turn-0')])
    await flush()

    expect(submit).toHaveBeenCalledTimes(2)
    expect(submit.mock.calls[1]?.[2]).toBe('two')
    expect(outbox().map((item) => [item.text, item.state])).toEqual([
      ['one', 'failed'],
      ['two', 'sending'],
    ])
  })

  it('keeps the prompt the provider already holds when Esc is refused', async () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', 'already gone')])
    seedFeed('c2', [ledger('c2').prompt('r2', 'still asking')])
    const answer = deferred<never>()
    retract.mockReturnValueOnce(answer.promise).mockReturnValueOnce(deferred<never>().promise)

    const taken = retractNewest('w1', 'c1')
    void retractNewest('w1', 'c2')
    expect(toasts.mock.calls.map(([toast]) => toast.type)).toEqual(['loading', 'loading'])
    answer.reject(
      new ApiError(
        'agent: retract: the prompt is already with its provider',
        409,
        'prompt_with_provider',
      ),
    )

    await expect(taken).resolves.toBeNull()
    expect(chatStore.getState().feeds.c1?.order).toEqual(['p:r1'])
    expect(closed.mock.calls).toEqual([[toasts.mock.calls[0]?.[0].id]])
    expect(toasts.mock.calls[1]?.[0].id).not.toBe(toasts.mock.calls[0]?.[0].id)
    expect(toasts.mock.calls[2]?.[0]).toEqual({
      type: 'error',
      title: 'Its provider already has this prompt',
      description: 'Stop ends its turn.',
    })
    expect(reported).not.toHaveBeenCalled()
  })

  it('says nothing when Esc lost the race to its prompt settling', async () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', 'done')])
    retract.mockImplementation(() => {
      applyFeedFrame('c1', { type: 'row', row: l.settle('r1') })
      return Promise.reject(new ApiError('prompt is settled: conflict', 409))
    })

    await expect(retractNewest('w1', 'c1')).resolves.toBeNull()

    expect(reported).not.toHaveBeenCalled()
    expect(toasts.mock.calls.map(([toast]) => toast.type)).toEqual(['loading'])
  })

  it('commits a staged pick only once the daemon took the prompt', async () => {
    seedFeed('c1', [])
    chatStore.setState({ chats: { c1: { id: 'c1', model: 'old', effort: 'low' } as never } })
    sendPrompt('w1', 'c1', { text: 'go', steer: false, model: 'big', effort: '' })
    expect(submit.mock.calls[0]?.slice(4)).toEqual(['', 'big', ''])
    await flush()
    expect(chatStore.getState().chats.c1).toMatchObject({ model: 'big', effort: '' })
  })

  it('Retry on a refused prompt row sends it again as a new request', async () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', 'again'), l.settle('r1', false, 'not_acknowledged')])

    retryPrompt('w1', 'c1', 'r1', false)

    expect(submit).toHaveBeenCalledTimes(1)
    expect(submit.mock.calls[0]?.[2]).toBe('again')
    expect(sentId()).not.toBe('r1')
    expect(chatStore.getState().feeds.c1?.order).toEqual(['p:r1'])
  })

  it('sends a Retry on a refused row once, however often it is pressed', () => {
    const l = ledger('c1')
    seedFeed('c1', [l.prompt('r1', 'again'), l.settle('r1', false, 'not_acknowledged')])
    const busyTurn = deferred<typeof accepted>()
    submit.mockReturnValue(busyTurn.promise)

    retryPrompt('w1', 'c1', 'r1', false)
    retryPrompt('w1', 'c1', 'r1', false)

    expect(submit).toHaveBeenCalledTimes(1)
    expect(outbox()).toHaveLength(1)
  })

  it('never resends a busy-refused send that Esc took back', async () => {
    const l = ledger('c1')
    seedFeed('c1', [])
    const sent = deferred<typeof accepted>()
    submit.mockReturnValueOnce(sent.promise)
    send('lost the race')
    retract.mockReturnValue(new Promise(() => {}))

    const restored = retractNewest('w1', 'c1')
    sent.reject(new ApiError('busy: conflict', 409, 'chat_busy'))
    await flush()
    seedFeed('c1', [l.start('turn-x'), l.end('turn-x')])

    await expect(restored).resolves.toBe('lost the race')
    expect(submit).toHaveBeenCalledTimes(1)
    expect(outbox()).toEqual([])
  })

  it('answers every Esc on the same in-flight send', async () => {
    seedFeed('c1', [])
    const sent = deferred<typeof accepted>()
    submit.mockReturnValueOnce(sent.promise)
    send('twice')
    retract.mockResolvedValue({ state: 'retracted', text: 'twice' })

    const first = retractNewest('w1', 'c1')
    const second = retractNewest('w1', 'c1')
    sent.resolve(accepted)

    await expect(first).resolves.toBe('twice')
    await expect(second).resolves.toBe('twice')
    expect(retract).toHaveBeenCalledTimes(1)
  })

  it('sends two queued prompts in order, each on its own turn_ended', async () => {
    const l = busy()
    send('one')
    send('two')
    seedFeed('c1', [l.end('turn-0')])
    await flush()
    expect(submit.mock.calls.map((call) => call[2])).toEqual(['one'])

    seedFeed('c1', [l.prompt(sentId(), 'one'), l.start('turn-1', sentId()), l.end('turn-1')])
    await flush()
    expect(submit.mock.calls.map((call) => call[2])).toEqual(['one', 'two'])
  })

  it('takes back only the newest prompt behind one that is sending', async () => {
    seedFeed('c1', [])
    submit.mockReturnValueOnce(new Promise(() => {}))
    send('one')
    send('two')

    await expect(retractNewest('w1', 'c1')).resolves.toBe('two')
    expect(outbox().map((item) => item.text)).toEqual(['one'])
  })

  it('retracts the recorded prompt when Esc meets a send of uncertain outcome', async () => {
    seedFeed('c1', [])
    submit.mockRejectedValueOnce(
      new ApiError('outcome uncertain: conflict', 409, 'request_outcome_uncertain'),
    )
    send('did it go')
    await flush()
    retract.mockResolvedValue({ state: 'retracted', text: 'did it go' })

    await expect(retractNewest('w1', 'c1')).resolves.toBe('did it go')
    expect(retract).toHaveBeenCalledWith('w1', 'c1', sentId())
  })

  it('holds the outbox while a terminal prompt waits on the chat', () => {
    const l = ledger('c1')
    seedFeed('c1', [l.start('turn-0'), l.end('turn-0')])
    chatStore.setState({ chats: { c1: { id: 'c1', terminalWait: { kind: 'trust' } } as never } })
    send('after the trust prompt')
    expect(submit).not.toHaveBeenCalled()

    act(() => chatStore.setState({ chats: { c1: { id: 'c1' } as never } }))
    expect(submit).toHaveBeenCalledTimes(1)
  })

  it('holds the outbox while the chat shows its Terminal, and sends on the way back', () => {
    const l = ledger('c1')
    seedFeed('c1', [l.start('turn-0')])
    const release = holdForTerminal('c1')
    send('typed in Chat')
    seedFeed('c1', [l.end('turn-0')])
    expect(submit).not.toHaveBeenCalled()

    release()
    expect(submit).toHaveBeenCalledTimes(1)
  })
})
