import { beforeAll, afterAll, afterEach, beforeEach, expect, test } from 'vitest'
import { http } from 'msw'
import { setupServer } from 'msw/node'
import { ok, fail } from '@/mocks/handlers/envelope'
import { startAgentRunForChat } from '@/features/markdown-chat/lib/start-run'
import { useSidebarStore } from '@/lib/store/sidebar'

// Regression for BUG-022: sending a chat message must drive the real v0
// agent-run contract (POST /v0/workspaces/:wsId/runs → POST /v0/runs/:id/start),
// not the retired /v0/markdown-chat mock route.

const requests: string[] = []

const server = setupServer(
  http.post('/v0/workspaces/:wsId/runs', async ({ params, request }) => {
    const body = (await request.json()) as { chatId: string }
    requests.push(`POST /v0/workspaces/${String(params.wsId)}/runs chatId=${body.chatId}`)
    return ok(
      {
        id: 'run-1',
        wsId: String(params.wsId),
        chatId: body.chatId,
        status: 'queued',
        createdAt: '2026-06-10T00:00:00Z',
      },
      201,
    )
  }),
  http.post('/v0/runs/:id/start', ({ params }) => {
    requests.push(`POST /v0/runs/${String(params.id)}/start`)
    return ok({
      id: String(params.id),
      wsId: 'ws-1',
      chatId: 'chat-1',
      status: 'running',
      createdAt: '2026-06-10T00:00:00Z',
    })
  }),
)

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

beforeEach(() => {
  requests.length = 0
  useSidebarStore.setState({ chats: [] })
  useSidebarStore.getState().addChat({
    id: 'chat-1',
    wsId: 'ws-1',
    title: 'Test chat',
    status: 'idle',
    type: 'chat',
    age: 'now',
  })
})

test('creates and starts a run via the real v0 endpoints', async () => {
  const runId = await startAgentRunForChat('chat-1')
  expect(runId).toBe('run-1')
  expect(requests).toEqual([
    'POST /v0/workspaces/ws-1/runs chatId=chat-1',
    'POST /v0/runs/run-1/start',
  ])
})

test('rejects when the chat is not linked to a backend chat', async () => {
  await expect(startAgentRunForChat('unknown-chat')).rejects.toThrow(/not linked to a backend chat/)
  expect(requests).toEqual([])
})

test('rejects when the run fails to create (provider/backend failure)', async () => {
  server.use(http.post('/v0/workspaces/:wsId/runs', () => fail('provider unavailable', 500)))
  await expect(startAgentRunForChat('chat-1')).rejects.toThrow('provider unavailable')
})

test('rejects when the run fails to start', async () => {
  server.use(http.post('/v0/runs/:id/start', () => fail('start failed', 500)))
  await expect(startAgentRunForChat('chat-1')).rejects.toThrow('start failed')
})
