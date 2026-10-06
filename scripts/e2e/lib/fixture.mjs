import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { STUB_PROVIDER_ID } from './stub-provider.mjs'
import { until } from './wait.mjs'

const PROJECT_NAME = 'E2E Project'
const REPO_NAME = 'e2e-repo'

function git(cwd, ...args) {
  // Pinned to the fixture: no inherited GIT_* can point git at another repository.
  const env = { ...process.env, GIT_CEILING_DIRECTORIES: join(cwd, '..') }
  for (const key of Object.keys(env))
    if (key.startsWith('GIT_') && key !== 'GIT_CEILING_DIRECTORIES') delete env[key]
  return execFileSync('git', args, { cwd, env, encoding: 'utf8' })
}

/** A throwaway git repository inside the isolated run dir (never inside the source checkout). */
export function createGitFixture(runDir) {
  const projectDir = join(runDir, 'project')
  const repoDir = join(projectDir, REPO_NAME)
  mkdirSync(repoDir, { recursive: true })
  git(repoDir, 'init', '-b', 'main')
  git(repoDir, 'config', 'user.name', 'E2E')
  git(repoDir, 'config', 'user.email', 'e2e@crowbar.local')
  writeFileSync(join(repoDir, 'README.md'), '# e2e fixture\n')
  mkdirSync(join(repoDir, 'src'))
  writeFileSync(join(repoDir, 'src', 'a.ts'), 'export const a = 1\n')
  git(repoDir, 'add', '-A')
  git(repoDir, 'commit', '-m', 'init')
  return { projectDir, repoDir }
}

const repoBase = (f) => `/v0/projects/${f.projectId}/repos/${f.repoId}`

/** Imports the fixture and returns the ids every scenario builds on. */
export async function importFixture(daemon, { projectDir, repoDir }) {
  await daemon.post('/v0/projects', { name: PROJECT_NAME, path: projectDir })
  const project = await until('the project to be listed', async () =>
    (await daemon.get('/v0/projects')).find((p) => p.name === PROJECT_NAME),
  )
  await daemon.post(`/v0/projects/${project.id}/repos`, {
    name: REPO_NAME,
    path: repoDir,
    defaultBranch: 'main',
  })
  const repo = await until('the repo to be listed', async () =>
    (await daemon.get(`/v0/projects/${project.id}/repos`)).find(
      (r) => r.name === REPO_NAME && r.projectId === project.id,
    ),
  )
  const fixture = { projectId: project.id, repoId: repo.id }
  const base = await until('the locked base workspace chat', async () =>
    (await daemon.get(`${repoBase(fixture)}/chats`)).find(
      (c) =>
        c.worktree?.branch === 'main' && !c.worktree.isDefault && c.worktree.status !== 'deleted',
    ),
  )
  return { ...fixture, baseChatId: base.id, base: repoBase(fixture) }
}

/** The stub provider must be listed and enabled before a chat can spawn on it. */
export async function waitStubProviderEnabled(daemon, fx) {
  await until('the stub provider to be enabled', async () =>
    (await daemon.get(`${fx.base}/chats/providers`)).find(
      (p) => p.id === STUB_PROVIDER_ID && p.enabled,
    ),
  )
}

/** A chat on the stub provider sharing `workspaceId`'s worktree. */
export async function createChat(daemon, fx, workspaceId) {
  const created = await daemon.post(`${fx.base}/chats`, {
    provider: STUB_PROVIDER_ID,
    workspaceId,
  })
  return awaitRunner(daemon, fx, created.id)
}

/** A chat on the stub provider forked into its OWN worktree (its own workspace). */
export async function createWorkspaceChat(daemon, fx) {
  const created = await daemon.post(`${fx.base}/chats`, {
    provider: STUB_PROVIDER_ID,
    parentId: fx.baseChatId,
    ownWorktree: true,
  })
  return awaitRunner(daemon, fx, created.id)
}

async function awaitRunner(daemon, fx, id) {
  const chat = await until(`chat ${id} to have a live runner`, async () => {
    const c = await daemon.get(`${fx.base}/chats/${id}`)
    return c.liveRunnerId ? c : null
  })
  return { id, workspaceId: chat.workspaceId, runnerId: chat.liveRunnerId }
}

export const getChat = (daemon, fx, id) => daemon.get(`${fx.base}/chats/${id}`)

/** Plays one provider hook at the daemon, exactly as the in-PTY hook command would. */
export async function hook(daemon, fx, chat, event, payload) {
  const res = await daemon.call('POST', `${fx.base}/chats/hooks`, {
    segment_id: chat.runnerId,
    provider: STUB_PROVIDER_ID,
    event,
    payload_raw: JSON.stringify(payload),
  })
  if (res.status !== 202)
    throw new Error(`hook ${event} -> ${res.status}: ${res.text.slice(0, 200)}`)
}

/** Like hook(), with a delivery id; returns the daemon's ack (its `reply` and `await`). */
export async function hookAck(daemon, fx, chat, event, payload) {
  const res = await daemon.call('POST', `${fx.base}/chats/hooks`, {
    segment_id: chat.runnerId,
    provider: STUB_PROVIDER_ID,
    event,
    payload_raw: JSON.stringify(payload),
    delivery_id: crypto.randomUUID(),
  })
  if (res.status !== 202)
    throw new Error(`hook ${event} -> ${res.status}: ${res.text.slice(0, 200)}`)
  return res.json?.data ?? {}
}
