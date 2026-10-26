import { useMemo } from 'react'
import { getBuildInfo } from '@/lib/build-info'
import type { BuildChannel, BuildInfo } from '@/lib/build-info'
import { useSettingsStore } from '@/features/settings/store'
import { useFrameStats, type FrameStats } from '@/lib/frame-stats'
import { useConnectionStore } from '@/lib/ws/connection-store'
import { useTerminalBufferCount } from '@/features/workspace/stores/hooks/use-buffer-store'
import {
  useOpenWorkspaceCount,
  useWorkingChatCount,
} from '@/features/workspace/stores/hooks/use-open-workspace-counts'
import type { NativeReadout } from '@/lib/native-chrome'
import {
  formatReadout,
  netStateOf,
  type NetState,
  type ReadoutLines,
} from '@/components/layout/build-readout-lines'

function previewInfo(channel: BuildChannel): BuildInfo {
  const timestamp = new Date().toISOString()
  if (channel === 'nightly') return { channel, timestamp }
  if (channel === 'beta') return { channel, version: '0.0.0-beta.1', timestamp }
  if (channel === 'release') return { channel, version: '0.0.0' }
  return { channel: 'dev', timestamp }
}

/** Settings override wins; 'auto' (the default) detects the real build. */
export function useResolvedBuildInfo(): BuildInfo | null {
  const override = useSettingsStore((s) => s.settings.buildBadgeOverride)
  if (override === 'off') return null
  if (!override || override === 'auto') return getBuildInfo()
  return previewInfo(override)
}

/** The readout's text and net reading from the connection and open-work stores. */
export function useReadoutLines(
  info: BuildInfo | null,
  frame: FrameStats,
): { lines: ReadoutLines; net: NetState } | null {
  const status = useConnectionStore((s) => s.status)
  const degraded = useConnectionStore((s) => s.degraded)
  const reconnects = useConnectionStore((s) => s.reconnects)
  const pty = useTerminalBufferCount()
  const ag = useWorkingChatCount()
  const ws = useOpenWorkspaceCount()
  if (!info) return null
  const net = netStateOf(status, degraded)
  return { net, lines: formatReadout({ info, net, reconnects, frame, counts: { pty, ag, ws } }) }
}

/** The readout's text for the phone's sidebar page, or null while it is turned off. */
export function useNativeReadout(): Pick<NativeReadout, 'lines' | 'net'> | null {
  const info = useResolvedBuildInfo()
  const visible = useSettingsStore((s) => s.settings.showBuildReadout)
  const frame = useFrameStats(visible)
  const parts = useReadoutLines(info, frame)
  // Keyed by value: the parts are rebuilt every render, and each change is a snapshot push.
  const key = visible && parts ? JSON.stringify(parts) : ''
  return useMemo(() => (key ? (JSON.parse(key) as NonNullable<typeof parts>) : null), [key])
}
