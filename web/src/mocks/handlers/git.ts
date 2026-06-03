import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const gitHandlers = [
  http.get('/api/v0/git/status', ({ request }) => {
    if (shouldFault(request, 'git-status'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.gitStatus(repo))
  }),

  http.get('/api/v0/git/log', ({ request }) => {
    if (shouldFault(request, 'git-commits'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const url = new URL(request.url)
    const repo = url.searchParams.get('repo') ?? ''
    const limit = Math.min(parseInt(url.searchParams.get('limit') ?? '50', 10), 500)
    const skip = parseInt(url.searchParams.get('skip') ?? '0', 10)
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.gitLog(repo).slice(skip, skip + limit))
  }),

  http.get('/api/v0/git/branches', ({ request }) => {
    if (shouldFault(request, 'git-branches'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.gitBranches(repo))
  }),
]
