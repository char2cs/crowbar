import { http, HttpResponse } from 'msw'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'

export const fsHandlers = [
  http.get('/api/v0/fs/tree', ({ request }) => {
    const root = new URL(request.url).searchParams.get('root') ?? ''
    return HttpResponse.json(getMockFileTree(root))
  }),

  http.get('/api/v0/fs/file', ({ request }) => {
    const path = new URL(request.url).searchParams.get('path') ?? ''
    return HttpResponse.json(getMockFileContent(path))
  }),
]
