import { http, HttpResponse } from 'msw'
import { getMockGitStatus, getMockCommitHistory, getMockBranches } from '@/lib/mock/git-data'

export const gitHandlers = [
  http.get('/api/v0/git/status', ({ request }) => {
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    return HttpResponse.json(getMockGitStatus(repo))
  }),
  http.get('/api/v0/git/log', ({ request }) => {
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    return HttpResponse.json(getMockCommitHistory(repo))
  }),
  http.get('/api/v0/git/branches', ({ request }) => {
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    return HttpResponse.json(getMockBranches(repo))
  }),
]
