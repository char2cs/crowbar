import { useId, useState } from 'react'
import { FolderOpen } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CreateSpaceDaemonRow } from './create-space-daemon-row'
import { ProjectIconMark } from './project-icon-mark'
import { IconPopover, type StagedIcon } from './icon-popover'
import { putIconFile } from './icon-upload'
import { getFilenameFromPath } from '@/features/file-system/controllers/file-utils'
import { apiFetch } from '@/lib/api/fetch'
import { createProject } from '@/lib/store/create-project'
import { report } from '@/lib/errors/report'
import { projectBaseOnHost } from '@/lib/project-url'
import { hostIsUnreachable } from '@/features/hosts/lib/host-tone'
import { useDaemonFolderPicker } from '@/features/hosts/hooks/use-daemon-folder-picker'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import { LOCAL_HOST, type HostDotState } from '@/features/hosts/types'
import type { Project } from '@/lib/types'

/** Why Create waits on a remote host, or null when it doesn't: the host first, then the folder. */
function remoteCreateHint(
  { name, state }: { name: string | undefined; state: HostDotState },
  selectedPath: string,
): string | null {
  if (name === undefined) return null
  if (state === 'needs-repair') return `${name} needs pairing again.`
  if (hostIsUnreachable(state)) return `${name} is not answering.`
  return selectedPath ? null : `Choose a folder on ${name} first.`
}

interface CreateSpacePanelProps {
  /** The daemon the form starts on; Local when absent. */
  defaultHostId?: string
  onCreate: (project: Project) => void
  onCancel: () => void
}

/** Applies a staged icon pick to the just-created project, on the three routes IconPopover would
 *  have hit. It is not in the project list yet, so the host it was created on is passed in. */
async function applyStagedIcon(
  hostId: string,
  projectId: string,
  staged: StagedIcon,
): Promise<void> {
  const base = projectBaseOnHost(hostId, projectId)
  switch (staged.kind) {
    case 'emoji':
      await apiFetch(`${base}/icon/emoji`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ emoji: staged.emoji }),
      })
      return
    case 'path':
      await putIconFile(`${base}/icon`, staged.path)
      return
    case 'file': {
      const form = new FormData()
      form.append('icon', staged.file)
      await apiFetch(`${base}/icon`, { method: 'PUT', body: form })
      return
    }
    case 'reset':
      // A brand-new project has no icon set yet — nothing to reset.
      return
  }
}

/** The "Create a Space" sheet, reached from the switcher's `+`, replacing SpaceScroller so a
 *  swipe never lands on it. Its icon pick is staged (no project id yet) and applied once the
 *  create settles, before `onCreate`: the new space arrives with its icon already set. */
export function CreateSpacePanel({
  defaultHostId = LOCAL_HOST.id,
  onCreate,
  onCancel,
}: CreateSpacePanelProps) {
  const [name, setName] = useState('')
  const [folder, setFolder] = useState<{ hostId: string; path: string } | null>(null)
  const [stagedIcon, setStagedIcon] = useState<StagedIcon | null>(null)
  const [loading, setLoading] = useState(false)
  const [hostId, setHostId] = useState(defaultHostId)

  const stagedEmoji = stagedIcon?.kind === 'emoji' ? stagedIcon.emoji : undefined
  const stagedIconUrl =
    stagedIcon?.kind === 'path' || stagedIcon?.kind === 'file' ? stagedIcon.previewUrl : undefined
  const selectedHost = useHostView(hostId)
  // Local keeps today's flow: only a remote that cannot be reached stops a create.
  const hostRefusesCreate = hostId !== LOCAL_HOST.id && hostIsUnreachable(selectedHost.state)
  // A folder names a path on the daemon it was picked on, and on no other.
  const selectedPath = folder?.hostId === hostId ? folder.path : ''
  const folderPicker = useDaemonFolderPicker(hostId, (path, pickedOn) =>
    setFolder({ hostId: pickedOn, path }),
  )
  const createHintId = useId()
  const createHint = hostId === LOCAL_HOST.id ? null : remoteCreateHint(selectedHost, selectedPath)

  async function handleCreate() {
    if (!selectedPath || loading || hostRefusesCreate) return
    setLoading(true)
    try {
      const trimmedName = name.trim() || getFilenameFromPath(selectedPath)
      const project = await createProject(trimmedName, selectedPath, hostId)
      if (stagedIcon) {
        // Best-effort: the space itself was created successfully, so a
        // failed icon mutation surfaces its own toast rather than blocking
        // entry to the new space.
        try {
          await applyStagedIcon(hostId, project.id, stagedIcon)
        } catch (err) {
          report(err, { action: 'Failed to set icon' })
        }
      }
      onCreate(project)
    } catch (err) {
      report(err, { action: 'Failed to create space' })
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      data-testid="create-space-panel"
      className="flex min-h-0 flex-1 flex-col gap-5 overflow-hidden px-3 pt-4"
    >
      <div>
        <h2 className="font-semibold text-base text-foreground">Create a Space</h2>
        <p className="mt-1 text-muted-foreground text-sm">
          Spaces are used to organize your tabs and sessions.
        </p>
      </div>

      <div className="flex items-center gap-2.5">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border border-dashed">
          <IconPopover
            name={name || 'Space'}
            emoji={stagedEmoji}
            iconUrl={stagedIconUrl}
            fallback={
              <ProjectIconMark
                project={{ name: name || 'Space', avatarUrl: undefined, avatarEmoji: undefined }}
                size="lg"
              />
            }
            fallbackLarge={
              <ProjectIconMark
                project={{ name: name || 'Space', avatarUrl: undefined, avatarEmoji: undefined }}
                size="xl"
              />
            }
            onStage={setStagedIcon}
          />
        </div>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Space Name"
          autoFocus
        />
      </div>

      <CreateSpaceDaemonRow hostId={hostId} onHostIdChange={setHostId} />

      <button
        type="button"
        onClick={folderPicker.pick}
        className="flex items-center justify-between rounded-lg border border-border bg-muted px-3 py-2.5 text-sm hover:bg-sidebar-element-hover"
      >
        <span className="flex items-center gap-2 text-foreground">
          <FolderOpen size={16} className="text-muted-foreground" />
          Folder
        </span>
        <span className="min-w-0 truncate text-muted-foreground">
          {selectedPath ? getFilenameFromPath(selectedPath) : 'Choose…'}
        </span>
      </button>

      {folderPicker.picker}

      <div className="flex-1" />

      <div className="flex shrink-0 flex-col gap-2 pb-4">
        {createHint && (
          <p id={createHintId} className="text-center text-muted-foreground text-xs">
            {createHint}
          </p>
        )}
        <Button
          onClick={() => void handleCreate()}
          disabled={!selectedPath || loading || hostRefusesCreate}
          aria-describedby={createHint ? createHintId : undefined}
          className="w-full"
        >
          {loading ? 'Creating…' : 'Create Space'}
        </Button>
        <Button variant="ghost" onClick={onCancel} className="w-full">
          Cancel
        </Button>
      </div>
    </div>
  )
}
