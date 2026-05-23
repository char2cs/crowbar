import type { WorkspacePayload } from '@/lib/types'
import { FEATURE_DEV_FLOW, MOCK_FLOWS } from './flows'

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
  const id = `ws-${Date.now()}`
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
