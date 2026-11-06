import { postProject } from '@/lib/api'
import { entityOf, entityStore } from '@/core/entities/entity-store'
import { runOperation, whenSettled } from '@/core/entities/run-operation'
import type { Project } from '@/lib/types'
import { UserFacingError } from '@/lib/errors/user-facing-error'

/** Adds the project at `path` on `hostId` as one operation and resolves with it once
 *  the projects store holds it; rejects with the operation's reason. */
export async function createProject(name: string, path: string, hostId: string): Promise<Project> {
  const op = runOperation(
    // The daemon names the project; its id arrives as the operation's item.
    {
      kind: 'project.create',
      subject: { kind: 'project', id: '' },
      projectId: '',
      // Quoted: a folder name alone ("missing") reads as part of the sentence.
      label: `Add “${name}”`,
    },
    (opId) => postProject(name, path, opId, hostId),
  )
  const settled = await whenSettled(op.id)
  const id = settled.items.find((item) => item.ref.kind === 'project')?.ref.id
  const project = id ? entityOf(entityStore.getState(), 'project', hostId, id) : undefined
  if (!project) throw new UserFacingError(`${settled.label}: the project is not in the list`)
  return project
}
