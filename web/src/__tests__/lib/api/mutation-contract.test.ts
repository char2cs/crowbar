import { afterAll, afterEach, beforeAll, describe, expect, test } from 'vitest'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { projectHandlers } from '@/mocks/handlers/projects'
import { workspaceHandlers } from '@/mocks/handlers/workspaces'
import { postProject, postWorkspace, fetchProject, apiFetch } from '@/lib/api'

// The real backend wraps every response in { success, error, data }. Mutations
// (POST) go through WriteMutationOK and return ONLY { id } in `data`, never the
// full entity. These tests pin both the mock handlers and the api client to
// that contract so the mocks can't silently diverge from production again.
const server = setupServer(...projectHandlers, ...workspaceHandlers)

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

describe('mutation responses are { id } only', () => {
  test('POST /v0/projects returns { id } and nothing else', async () => {
    const res = await postProject('my-proj', '/tmp/my-proj')
    expect(Object.keys(res)).toEqual(['id'])
    expect(typeof res.id).toBe('string')
    expect(res.id.length).toBeGreaterThan(0)
  })

  test('fetchProject re-fetches the full entity created by the POST mock', async () => {
    const { id } = await postProject('hydrate-me', '/tmp/hydrate-me')
    const project = await fetchProject(id)
    expect(project.id).toBe(id)
    expect(project.name).toBe('hydrate-me')
    expect(project.path).toBe('/tmp/hydrate-me')
  })

  test('GET /v0/projects/:id returns 404 envelope for unknown id', async () => {
    await expect(fetchProject('does-not-exist')).rejects.toThrow('not found')
  })

  test('POST /v0/workspaces returns { id } only', async () => {
    const res = await postWorkspace('crowbar', 'feature/test')
    expect(Object.keys(res)).toEqual(['id'])
    expect(typeof res.id).toBe('string')
  })
})

describe('apiFetch empty/204 success handling', () => {
  test('treats a 204 No Content success as undefined, not an error', async () => {
    server.use(
      http.post('/v0/things/delete', () => new HttpResponse(null, { status: 204 })),
    )
    await expect(apiFetch('/v0/things/delete', { method: 'POST' })).resolves.toBeUndefined()
  })

  test('treats an empty-body 200 success as undefined', async () => {
    server.use(
      http.post('/v0/things/noop', () => new HttpResponse(null, { status: 200 })),
    )
    await expect(apiFetch('/v0/things/noop', { method: 'POST' })).resolves.toBeUndefined()
  })

  test('still throws on a real error envelope', async () => {
    server.use(
      http.get('/v0/things/boom', () =>
        HttpResponse.json({ success: false, error: 'kaboom', data: null }, { status: 500 }),
      ),
    )
    await expect(apiFetch('/v0/things/boom')).rejects.toThrow('kaboom')
  })
})
