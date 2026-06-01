import { http, HttpResponse } from 'msw'
import {
  getMockBranchDiff,
  getMockBranchReviewThreads,
  getMockBranchReviewDescription,
  getMockBranchReviewChats,
} from '@/lib/mock/branch-diff'

export const branchReviewHandlers = [
  http.get('/api/v0/branch-review/:wsId/diff', ({ params }) =>
    HttpResponse.json(getMockBranchDiff(params.wsId as string))
  ),
  http.get('/api/v0/branch-review/:wsId/threads', ({ params }) =>
    HttpResponse.json(getMockBranchReviewThreads(params.wsId as string))
  ),
  http.get('/api/v0/branch-review/:wsId/description', ({ params }) =>
    HttpResponse.json(getMockBranchReviewDescription(params.wsId as string))
  ),
  http.get('/api/v0/branch-review/:wsId/chats', ({ params }) =>
    HttpResponse.json(getMockBranchReviewChats(params.wsId as string))
  ),
]
