import { nanoid } from 'nanoid'
import type { WorkspacePayload } from '@/lib/types'
import { FEATURE_DEV_FLOW, MOCK_FLOWS } from './flows'
import type { Repo } from '@/lib/store/sidebar'

const INITIAL_WORKSPACES: WorkspacePayload[] = [
  {
    id: 'ws3', repoId: 'crowbar', branch: 'feature/app-design',
    flowName: 'feature-development', currentState: 'brainstorming',
    flow: FEATURE_DEV_FLOW,
  },
  {
    id: 'ws2', repoId: 'crowbar', branch: 'feature/api-backend',
    flowName: 'feature-development', currentState: 'implementation',
    flow: FEATURE_DEV_FLOW,
  },
  {
    id: 'ws1', repoId: 'crowbar', branch: 'enhancement/scaffold',
    flowName: 'feature-development', currentState: 'human_review',
    flow: FEATURE_DEV_FLOW,
  },
  {
    id: 'qc1', repoId: 'quiver-core', branch: 'develop',
    flowName: 'feature-development', currentState: 'brainstorming',
    flow: FEATURE_DEV_FLOW,
  },
  {
    id: 'qd1', repoId: 'quiver-desktop', branch: 'develop',
    flowName: 'feature-development', currentState: 'brainstorming',
    flow: FEATURE_DEV_FLOW,
  },
  {
    id: 'qd2', repoId: 'quiver-desktop', branch: 'feature/quiver-shell',
    flowName: 'feature-development', currentState: 'spec',
    flow: FEATURE_DEV_FLOW,
  },
]

const store = new Map<string, WorkspacePayload>(
  INITIAL_WORKSPACES.map(ws => [ws.id, ws]),
)

export function getMockWorkspace(wsId: string): WorkspacePayload | undefined {
  return store.get(wsId)
}

export function createMockWorkspace(
  repoId: string,
  branch: string,
  flowName: string,
): WorkspacePayload {
  const flow = MOCK_FLOWS.find(f => f.name === flowName) ?? FEATURE_DEV_FLOW
  const id = nanoid()
  const ws: WorkspacePayload = {
    id, repoId, branch, flowName,
    currentState: flow.states[0].name,
    flow,
  }
  store.set(id, ws)
  return ws
}

export function deleteMockWorkspace(id: string): void {
  store.delete(id)
}

export function getMockRepos(): Repo[] {
  return [
    {
      id: 'crowbar', name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-indigo-700',
      workspaces: [
        { id: 'ws-develop', branch: 'develop', status: 'locked', age: '—' },
        { id: 'ws3', branch: 'feature/app-design', parentId: 'ws-develop', status: 'pr-open', added: 5672, age: '16h ago' },
        { id: 'ws1', branch: 'enhancement/scaffold', parentId: 'ws3', status: 'agent-running', added: 22892, age: '3d ago' },
        { id: 'ws-fix', branch: 'fix/toolbar-crash', parentId: 'ws3', status: 'new', age: 'just now' },
        { id: 'ws2', branch: 'feature/api-backend', parentId: 'ws-develop', status: 'pr-merged', added: 27347, deleted: 455, age: '1d ago' },
        { id: 'ws4', branch: 'feature/ws-channels', parentId: 'ws-develop', status: 'pr-open', added: 8841, deleted: 203, age: '2d ago' },
        { id: 'ws5', branch: 'refactor/query-layer', parentId: 'ws-develop', status: 'agent-running', added: 103482, deleted: 88910, age: '5d ago', hasConflicts: true },
        { id: 'ws6', branch: 'chore/bump-deps', parentId: 'ws-develop', status: 'pr-closed', added: 312, deleted: 298, age: '6d ago' },
      ],
    },
    {
      id: 'quiver-core', name: 'quiver.core', avatarLabel: 'Q', avatarColor: 'bg-emerald-700',
      workspaces: [
        { id: 'qc-develop', branch: 'develop', status: 'locked', age: '—' },
        { id: 'qc1', branch: 'feature/old-auth', parentId: 'qc-develop', status: 'pr-closed', age: '3d ago' },
        { id: 'qc2', branch: 'feature/oauth2', parentId: 'qc-develop', status: 'pr-open', added: 4521, deleted: 89, age: '1d ago' },
        { id: 'qc3', branch: 'fix/token-expiry', parentId: 'qc2', status: 'new', added: 47, age: 'just now', hasConflicts: true },
        { id: 'qc4', branch: 'perf/redis-cache', parentId: 'qc-develop', status: 'agent-running', added: 1823, deleted: 402, age: '12h ago' },
      ],
    },
    {
      id: 'quiver-desktop', name: 'quiver.desktop', avatarLabel: 'Q', avatarColor: 'bg-orange-700',
      workspaces: [
        { id: 'qd-develop', branch: 'develop', status: 'locked', age: '—' },
        { id: 'qd2', branch: 'feature/quiver-shell', parentId: 'qd-develop', status: 'pr-open', added: 13485, deleted: 69, age: '3d ago' },
        { id: 'qd3', branch: 'feat/native-file-picker', parentId: 'qd-develop', status: 'agent-running', added: 2341, age: '8h ago' },
        { id: 'qd4', branch: 'fix/startup-crash', parentId: 'qd-develop', status: 'pr-merged', added: 23, deleted: 7, age: '4d ago' },
      ],
    },
    {
      id: 'quiver-cloud', name: 'quiver.cloud', avatarLabel: 'Q', avatarColor: 'bg-sky-700',
      workspaces: [
        { id: 'qcl-develop', branch: 'develop', status: 'locked', age: '—' },
        { id: 'qcl1', branch: 'feature/multi-tenant', parentId: 'qcl-develop', status: 'pr-open', added: 31204, deleted: 1823, age: '2d ago' },
        { id: 'qcl2', branch: 'feat/s3-presign', parentId: 'qcl-develop', status: 'new', added: 892, age: '4h ago' },
        { id: 'qcl3', branch: 'chore/infra-terraform', parentId: 'qcl-develop', status: 'pr-merged', added: 4812, deleted: 3201, age: '7d ago' },
      ],
    },
  ]
}
