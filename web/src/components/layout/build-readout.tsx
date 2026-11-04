import { Fragment, type ReactNode } from 'react'
import type { BuildInfo } from '@/lib/build-info'
import { useSettingsStore } from '@/features/settings/store'
import { cn } from '@/utils/cn'
import { hasNativeChrome, type NativeReadout } from '@/lib/native-chrome'
import { useFrameStats, type FrameStats } from '@/lib/frame-stats'
import { useHostDotState } from '@/features/hosts/hooks/use-host-status'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { useResolvedBuildInfo, useReadoutLines } from '@/components/layout/use-readout-lines'
import {
  formatHostEntry,
  hostNetStateOf,
  type HostNetState,
  type NetState,
  type ReadoutLines,
} from '@/components/layout/build-readout-lines'

const READOUT_INK = '#E0E0D0'
const SEP = '  '
// A segment never breaks inside; the first line wraps only between segments.
const SEGMENT = 'whitespace-nowrap'

const NET_INK: Record<HostNetState, string | undefined> = {
  ok: undefined,
  idle: undefined,
  reconnecting: 'text-warning',
  repair: 'text-warning',
  down: 'text-destructive',
}

function HostEntryText({ name, net }: { name: string; net: HostNetState }) {
  return (
    <span data-host-net={net} className={cn(SEGMENT, NET_INK[net])}>
      {formatHostEntry(name, net)}
    </span>
  )
}

function HostEntry({ hostId, name }: { hostId: string; name: string }) {
  return <HostEntryText name={name} net={hostNetStateOf(useHostDotState(hostId))} />
}

/** Each paired remote host's state, after Local's own net reading; a local-only install has none. */
function HostEntries() {
  const hosts = useHostConfigStore((s) => s.hosts)
  return (
    <>
      {hosts.map((host) => (
        <Fragment key={host.id}>
          {SEP}
          <HostEntry hostId={host.id} name={host.name} />
        </Fragment>
      ))}
    </>
  )
}

/** The two readout lines, the hosts' entries after Local's own net reading. */
function ReadoutText({
  lines,
  net,
  hosts,
}: {
  lines: ReadoutLines
  net: NetState
  hosts: ReactNode
}) {
  return (
    <>
      <span>
        <span className={SEGMENT}>{lines.build}</span>
        {SEP}
        <span data-net-state={net} className={cn(SEGMENT, lines.netWarn && NET_INK[net])}>
          {lines.net}
        </span>
        {lines.reconnects && SEP + lines.reconnects}
        {hosts}
      </span>
      <span>{lines.stats}</span>
    </>
  )
}

/** The readout's ink and type, unpositioned: the phone's sidebar page places it itself. */
function ReadoutFace({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <div
      aria-hidden="true"
      style={{ color: READOUT_INK }}
      className={cn(
        'pointer-events-none z-50 flex select-none flex-col whitespace-pre-wrap font-mono text-[10px] leading-3 tabular-nums [text-shadow:0_0_2px_#000,0_1px_1px_#000,1px_0_1px_#000]',
        className,
      )}
    >
      {children}
    </div>
  )
}

/** The readout the phone's shell sent over, drawn by the sidebar page where it places it. */
export function NativeReadoutView({
  readout,
  className,
}: {
  readout: NativeReadout
  className?: string
}) {
  const hosts = readout.hosts.map((host) => (
    <Fragment key={host.name}>
      {SEP}
      <HostEntryText name={host.name} net={host.net} />
    </Fragment>
  ))
  return (
    <ReadoutFace className={className}>
      <ReadoutText lines={readout.lines} net={readout.net} hosts={hosts} />
    </ReadoutFace>
  )
}

function Lines({ info, frame }: { info: BuildInfo; frame: FrameStats }) {
  const parts = useReadoutLines(info, frame)
  if (!parts) return null
  return <ReadoutText lines={parts.lines} net={parts.net} hosts={<HostEntries />} />
}

/**
 * Build, daemon connection and frame stats, set like a net-graph: pale yellow
 * monospace with a dark outline, straight on the chrome. Purely visual: it
 * takes no layout space and no pointer events, so a tab under it stays
 * clickable. It is as wide as its text up to 42ch, then wraps between segments,
 * growing down from its anchor (`top-auto bottom-0` grows up); an anchor with a
 * real box (the sidebar foot) passes `w-auto` and both side insets.
 * On the phone the sidebar page draws it instead (`NativeReadoutView`).
 * The frame loop and the store watchers run only while this is shown
 * (Settings → Appearance → Build readout).
 */
export function BuildReadout({ className }: { className?: string }) {
  const info = useResolvedBuildInfo()
  const visible = useSettingsStore((s) => s.settings.showBuildReadout)
  const phone = hasNativeChrome()
  const frame = useFrameStats(visible && !phone)
  if (!visible || !info || phone) return null

  return (
    <ReadoutFace
      className={cn(
        'absolute top-0 min-h-full w-max max-w-[42ch] items-start justify-center',
        className,
      )}
    >
      <Lines info={info} frame={frame} />
    </ReadoutFace>
  )
}
