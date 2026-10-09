import { useEffect, useState } from 'react'
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
import { getFilenameFromPath } from '@/features/file-system/controllers/file-utils'
import { toast } from '@/features/window/stores/toast-store'
import { openNativeDialog as openDialog } from '@/lib/native-dialog'
import { apiFetch, postProject } from '@/lib/api'
import { awaitEntity } from '@/lib/ws/await-entity'
import { projectBaseOnHost } from '@/lib/project-url'
import { projectFromDTO } from '@/lib/store/project-from-dto'
import { cn } from '@/lib/utils'
import { AddHostDialog } from '@/features/hosts/components/add-host-dialog'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST, type Host, type HostStatus } from '@/features/hosts/types'
import type { Project, ProjectDTO } from '@/lib/types'

// A trailing pick that opens AddHostDialog rather than ever becoming the
// selected daemon — `handleHostIdChange` intercepts it before `setHostId`.
const ADD_HOST_VALUE = '__add-host__'

// Same labels/colors as HostsSettings' own status dot (hosts-settings.tsx) —
// no shared component to import yet, so inlined identically per this task's
// brief.
const DAEMON_STATUS_LABEL: Record<HostStatus, string> = {
  unknown: 'Status unknown',
  probing: 'Checking…',
  up: 'Online',
  down: 'Offline',
  'needs-repair': 'Needs repair — reconnect this host',
}

const DAEMON_STATUS_DOT_CLASS: Record<HostStatus, string> = {
  unknown: 'border border-muted-foreground/50 bg-transparent',
  probing: 'border border-muted-foreground/50 bg-transparent',
  up: 'bg-success',
  down: 'bg-destructive',
  'needs-repair': 'bg-warning',
}

/** The local daemon is always reachable from its own UI — there is nothing to
 *  probe, so its dot never leaves 'up' and it is never disabled. */
function useHostRuntimeStatus(hostId: string): HostStatus {
  const [status, setStatus] = useState<HostStatus>(() =>
    hostId === 'local' ? 'up' : getOrCreateHostRuntime(hostId).getState().status,
  )
  useEffect(() => {
    if (hostId === 'local') return
    const runtime = getOrCreateHostRuntime(hostId)
    setStatus(runtime.getState().status)
    return runtime.subscribe((s) => setStatus(s.status))
  }, [hostId])
  return status
}

function DaemonStatusDot({ status }: { status: HostStatus }) {
  return (
    <span
      aria-label={DAEMON_STATUS_LABEL[status]}
      title={DAEMON_STATUS_LABEL[status]}
      className={cn('size-2 shrink-0 rounded-full', DAEMON_STATUS_DOT_CLASS[status])}
    />
  )
}

/** One selectable daemon row: Local or a saved host, disabled (not hidden)
 *  while its runtime status is 'down' — a project can't be created on a host
 *  that can't currently be reached. */
function DaemonOption({ host }: { host: Host }) {
  const status = useHostRuntimeStatus(host.id)
  return (
    <SelectItem value={host.id} disabled={status === 'down'}>
      <span className="flex items-center gap-2">
        <DaemonStatusDot status={status} />
        {host.name}
      </span>
    </SelectItem>
  )
}

interface CreateSpacePanelProps {
  onCreate: (project: Project) => void
  onCancel: () => void
}

/** Applies a staged icon pick against a real, just-created project — the
 *  exact same three routes IconPopover itself would have hit had the space
 *  already existed when the pick was made. The project is not in the project
 *  list yet, so the host it was created on is passed rather than looked up. */
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
      await apiFetch(`${base}/icon`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: staged.path }),
      })
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
  const hosts = useHostConfigStore((s) => s.hosts)

  const stagedEmoji = stagedIcon?.kind === 'emoji' ? stagedIcon.emoji : undefined
  const stagedIconUrl =
    stagedIcon?.kind === 'path' || stagedIcon?.kind === 'file' ? stagedIcon.previewUrl : undefined
  const selectedHost =
    hostId === 'local' ? LOCAL_HOST : (hosts.find((h) => h.id === hostId) ?? LOCAL_HOST)
  const selectedHostStatus = useHostRuntimeStatus(selectedHost.id)

  async function handleBrowse() {
    const selected = await openDialog({ directory: true, multiple: false })
    if (typeof selected === 'string') setSelectedPath(selected)
  }

  function handleHostIdChange(value: string | null) {
    if (!value) return
    if (value === ADD_HOST_VALUE) {
      setAddHostOpen(true)
      return
    }
    setHostId(value)
  }

  async function handleCreate() {
    if (!selectedPath || loading) return
    setLoading(true)
    try {
      // Same subscribe-before-POST shape as the retired ImportProjectModal:
      // postProject answers 202 with no body, so the real Project (with its
      // daemon-assigned id) is read back off the /v0/projects WS stream,
      // matched by the path we just submitted.
      const trimmedName = name.trim() || getFilenameFromPath(selectedPath)
      const dto = await awaitEntity<ProjectDTO>({
        endpoint: '/v0/projects',
        match: (p) => p.path === selectedPath && p.status !== 'deleted',
        action: () => postProject(trimmedName, selectedPath, hostId),
      })
      if (stagedIcon) {
        // Best-effort: the space itself was created successfully, so a
        // failed icon mutation surfaces its own toast rather than blocking
        // entry to the new space.
        try {
          await applyStagedIcon(hostId, dto.id, stagedIcon)
        } catch {
          toast.error('Failed to set icon')
        }
      }
      onCreate(projectFromDTO(dto, hostId))
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create space')
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
                <DaemonStatusDot status={selectedHostStatus} />
                {selectedHost.name}
              </span>
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="local">
              <span className="flex items-center gap-2">
                <DaemonStatusDot status="up" />
                Local
              </span>
            </SelectItem>
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
        onClick={() => void handleBrowse()}
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

      <AddHostDialog open={addHostOpen} onOpenChange={setAddHostOpen} />

      <div className="flex-1" />

      <div className="flex shrink-0 flex-col gap-2 pb-4">
        <Button
          onClick={() => void handleCreate()}
          disabled={!selectedPath || loading}
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
