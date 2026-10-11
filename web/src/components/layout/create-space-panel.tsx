import { useId, useState } from 'react'
import { FolderOpen, HardDrives } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectPrimitive,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ProjectIconMark } from './project-icon-mark'
import { IconPopover, type StagedIcon } from './icon-popover'
import { putIconFile } from './icon-upload'
import { getFilenameFromPath } from '@/features/file-system/controllers/file-utils'
import { toast } from '@/features/window/stores/toast-store'
import { toastActionFailure, toastHostFailure } from '@/features/hosts/lib/host-failure-notice'
import { apiFetch, postProject } from '@/lib/api'
import { awaitEntity } from '@/lib/ws/await-entity'
import { qualify } from '@/lib/host-path'
import { projectBaseOnHost } from '@/lib/project-url'
import { projectFromDTO } from '@/lib/store/project-from-dto'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'
import {
  HostStatusDot,
  hostDotLabel,
  hostIsUnreachable,
} from '@/features/hosts/components/host-status-dot'
import { useDaemonFolderPicker } from '@/features/hosts/hooks/use-daemon-folder-picker'
import { useHostDotState, useHostView } from '@/features/hosts/hooks/use-host-status'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST, type Host } from '@/features/hosts/types'
import type { Project, ProjectDTO } from '@/lib/types'

// A trailing pick that opens AddHostDialog rather than ever becoming the
// selected daemon — `handleHostIdChange` intercepts it before `setHostId`.
const ADD_HOST_VALUE = '__add-host__'

/** One selectable saved host, disabled (not hidden) while nothing can be asked of it — a
 *  project can't be created on a host that can't currently be reached. */
function DaemonOption({ host }: { host: Host }) {
  const state = useHostDotState(host.id)
  return (
    <SelectItem value={host.id} disabled={hostIsUnreachable(state)}>
      <span className="flex items-center gap-2">
        <HostStatusDot hostId={host.id} state={state} />
        {host.name}
      </span>
    </SelectItem>
  )
}

function LocalOption() {
  const state = useHostDotState(LOCAL_HOST.id)
  return (
    <SelectItem value={LOCAL_HOST.id}>
      <span className="flex items-center gap-2">
        <HostStatusDot hostId={LOCAL_HOST.id} state={state} />
        {LOCAL_HOST.name}
      </span>
    </SelectItem>
  )
}

interface CreateSpacePanelProps {
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

/**
 * Zen Browser's "Create a Space" sheet (spec: same name+icon row, a
 * folder-picker row standing in for its Profile row, no theme row), reached
 * only via the `+` mark in SidebarFooter — it replaces SpaceScroller
 * entirely rather than joining its scroll-snap track, so the touchpad swipe
 * between real spaces never lands here by accident.
 *
 * The icon square is the real `IconPopover` — same Upload/Emoji/Reset sheet
 * every other project and repo icon opens — in its `onStage` mode: a pick is
 * held locally (`stagedIcon`) and fed straight back in as `emoji`/`iconUrl`
 * for the live preview, since there is no project id yet to mutate against.
 * `applyStagedIcon` persists it for real right after `postProject` answers,
 * before `onCreate` fires — the new space arrives with its icon already set,
 * not as a follow-up edit.
 */
export function CreateSpacePanel({ onCreate, onCancel }: CreateSpacePanelProps) {
  const [name, setName] = useState('')
  const [selectedPath, setSelectedPath] = useState('')
  const [stagedIcon, setStagedIcon] = useState<StagedIcon | null>(null)
  const [loading, setLoading] = useState(false)
  const [hostId, setHostId] = useState('local')
  const [addHostOpen, setAddHostOpen] = useState(false)
  // Each opening is a new attempt, even one made while the last is still closing.
  const [addHostAttempt, setAddHostAttempt] = useState(0)
  const hosts = useHostConfigStore((s) => s.hosts)

  const stagedEmoji = stagedIcon?.kind === 'emoji' ? stagedIcon.emoji : undefined
  const stagedIconUrl =
    stagedIcon?.kind === 'path' || stagedIcon?.kind === 'file' ? stagedIcon.previewUrl : undefined
  const selectedHost = useHostView(hostId)
  // Local keeps today's flow: only a remote that cannot be reached stops a create.
  const hostRefusesCreate = hostId !== LOCAL_HOST.id && hostIsUnreachable(selectedHost.state)
  const folderPicker = useDaemonFolderPicker(hostId, setSelectedPath)
  const folderHintId = useId()
  const folderHint =
    hostId !== LOCAL_HOST.id && !selectedPath && selectedHost.name !== undefined
      ? `Choose a folder on ${selectedHost.name} first.`
      : null

  function handleHostIdChange(value: string | null) {
    if (!value) return
    if (value === ADD_HOST_VALUE) {
      setAddHostAttempt((n) => n + 1)
      setAddHostOpen(true)
      return
    }
    // A folder names a path on the daemon it was picked on, and on no other.
    if (value !== hostId) setSelectedPath('')
    setHostId(value)
  }

  async function handleCreate() {
    if (!selectedPath || loading || hostRefusesCreate) return
    setLoading(true)
    try {
      // Same subscribe-before-POST shape as the retired ImportProjectModal:
      // postProject answers 202 with no body, so the real Project (with its
      // daemon-assigned id) is read back off the /v0/projects WS stream of the
      // host it is created on, matched by the path we just submitted — so the
      // DTO stamped with that host really came from it.
      const trimmedName = name.trim() || getFilenameFromPath(selectedPath)
      const dto = await awaitEntity<ProjectDTO>({
        endpoint: qualify(hostId, '/v0/projects'),
        match: (p) => p.path === selectedPath && p.status !== 'deleted',
        action: () => postProject(trimmedName, selectedPath, hostId),
      })
      if (stagedIcon) {
        // Best-effort: the space itself was created successfully, so a
        // failed icon mutation surfaces its own toast rather than blocking
        // entry to the new space.
        try {
          await applyStagedIcon(hostId, dto.id, stagedIcon)
        } catch (err) {
          if (!toastHostFailure(err)) toast.error('Failed to set icon')
        }
      }
      onCreate(projectFromDTO(dto, hostId))
    } catch (err) {
      toastActionFailure(err, 'Failed to create space')
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

      <div className="flex items-center justify-between rounded-lg border border-border bg-muted px-3 py-2.5 text-sm">
        <span className="flex items-center gap-2 text-foreground">
          <HardDrives size={16} className="text-muted-foreground" />
          Daemon
        </span>
        <Select value={hostId} onValueChange={handleHostIdChange}>
          <SelectTrigger
            aria-label="Daemon"
            size="sm"
            className="h-auto min-h-0 w-auto min-w-0 border-none bg-transparent px-0 text-muted-foreground shadow-none before:hidden hover:bg-transparent"
          >
            <SelectValue>
              <span className="flex items-center gap-1.5">
                <HostStatusDot hostId={hostId} state={selectedHost.state} />
                {selectedHost.name ?? hostDotLabel(selectedHost.state)}
              </span>
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <LocalOption />
            {hosts.length > 0 && <SelectPrimitive.Separator className="my-1 h-px bg-border" />}
            {hosts.map((host) => (
              <DaemonOption key={host.id} host={host} />
            ))}
            <SelectPrimitive.Separator className="my-1 h-px bg-border" />
            <SelectItem value={ADD_HOST_VALUE}>+ Add a host…</SelectItem>
          </SelectContent>
        </Select>
      </div>

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
      <AddHostDialog key={addHostAttempt} open={addHostOpen} onOpenChange={setAddHostOpen} />

      <div className="flex-1" />

      <div className="flex shrink-0 flex-col gap-2 pb-4">
        {folderHint && (
          <p id={folderHintId} className="text-center text-muted-foreground text-xs">
            {folderHint}
          </p>
        )}
        <Button
          onClick={() => void handleCreate()}
          disabled={!selectedPath || loading || hostRefusesCreate}
          aria-describedby={folderHint ? folderHintId : undefined}
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
