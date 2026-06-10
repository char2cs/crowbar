import {
  getOrCreateConversationStore,
  destroyConversationStore,
} from '@/features/markdown-chat/stores/conversation-store'

const TURN = {
  id: 't1',
  role: 'user' as const,
  content: 'Hello',
  timestamp: '2026-05-31T00:00:00Z',
  authorName: 'Mateo',
  widgets: [],
}

afterEach(() => {
  destroyConversationStore('ws1')
})

test('appendTurn adds turn to store', () => {
  const store = getOrCreateConversationStore('ws1')
  store.getState().appendTurn(TURN)
  expect(store.getState().turns).toHaveLength(1)
  expect(store.getState().turns[0].id).toBe('t1')
})

test('updateStreamingTurn appends content delta', () => {
  const store = getOrCreateConversationStore('ws1')
  store.getState().appendTurn({ ...TURN, id: 'a1', role: 'agent', streaming: true, content: '' })
  store.getState().updateStreamingTurn('a1', 'Hello ')
  store.getState().updateStreamingTurn('a1', 'world')
  expect(store.getState().turns[0].content).toBe('Hello world')
})

test('finalizeStreamingTurn clears streaming flag', () => {
  const store = getOrCreateConversationStore('ws1')
  store.getState().appendTurn({ ...TURN, id: 'a1', role: 'agent', streaming: true, content: '' })
  store.getState().finalizeStreamingTurn('a1')
  expect(store.getState().turns[0].streaming).toBe(false)
})

test('updateWidgetPayload updates widget in turn', () => {
  const store = getOrCreateConversationStore('ws1')
  store.getState().appendTurn({
    ...TURN,
    id: 'a1',
    widgets: [{ id: 'w1', type: 'excalidraw', payload: null }],
  })
  store.getState().updateWidgetPayload('a1', 'w1', { elements: [] })
  const widget = store.getState().turns[0].widgets[0]
  expect((widget.payload as { elements: unknown[] }).elements).toEqual([])
})

test('updateWidgetPayload is a no-op when turnId does not match', () => {
  const store = getOrCreateConversationStore('ws1')
  store
    .getState()
    .appendTurn({ ...TURN, widgets: [{ id: 'w1', type: 'excalidraw', payload: null }] })
  const before = store.getState().turns
  store.getState().updateWidgetPayload('nonexistent', 'w1', { elements: [] })
  expect(store.getState().turns).toEqual(before)
})

test('getOrCreateConversationStore returns same instance for same wsId', () => {
  const a = getOrCreateConversationStore('ws1')
  const b = getOrCreateConversationStore('ws1')
  expect(a).toBe(b)
})

test('destroyConversationStore removes the store', () => {
  const a = getOrCreateConversationStore('ws1')
  destroyConversationStore('ws1')
  const b = getOrCreateConversationStore('ws1')
  expect(a).not.toBe(b)
})
