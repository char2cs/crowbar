import { http, HttpResponse } from 'msw'
import { getMockGitStatus, getMockCommitHistory, getMockBranches } from '@/lib/mock/git-data'

export const gitHandlers = [
  http.get('/api/v0/git/status', ({ request }) => {
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    return HttpResponse.json(getMockGitStatus(repo))
  }),
  http.get('/api/v0/git/log', ({ request }) => {
    const url = new URL(request.url)
    const repo = url.searchParams.get('repo') ?? ''
    const limit = Math.min(parseInt(url.searchParams.get('limit') ?? '50', 10), 200)
    const skip = parseInt(url.searchParams.get('skip') ?? '0', 10)
    const all = getMockCommitHistory(repo)
    return HttpResponse.json(all.slice(skip, skip + limit))
  }),
  http.get('/api/v0/git/branches', ({ request }) => {
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    return HttpResponse.json(getMockBranches(repo))
  }),
]
