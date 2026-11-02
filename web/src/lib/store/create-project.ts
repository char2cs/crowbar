import { postProject } from '@/lib/api'
import { entityStore } from '@/core/entities/entity-store'
import { runOperation, whenSettled } from '@/core/entities/run-operation'
import type { Project } from '@/lib/types'

/** Adds the project at `path` as one operation and resolves with it once the
 *  projects store holds it; rejects with the operation's reason. */
export async function createProject(name: string, path: string): Promise<Project> {
  const op = runOperation(
    // The daemon names the project; its id arrives as the operation's item.
    {
      kind: 'project.create',
      subject: { kind: 'project', id: '' },
      projectId: '',
      label: `Add ${name}`,
    },
    (opId) => postProject(name, path, opId),
  )
  const settled = await whenSettled(op.id)
  const id = settled.items.find((item) => item.ref.kind === 'project')?.ref.id
  const project = id ? entityStore.getState().byKind.project[id]?.data : undefined
  if (!project) throw new Error(`${settled.label}: the project is not in the list`)
  return project
}
