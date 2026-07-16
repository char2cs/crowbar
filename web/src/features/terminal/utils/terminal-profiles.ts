import type { Shell, TerminalProfile } from '../types/terminal'

export const SYSTEM_DEFAULT_PROFILE_ID = 'system-default'
export const DEFAULT_SHELL_OPTION_VALUE = 'system'
export const DEFAULT_PROFILE_LABEL = 'Default Terminal'

export interface ResolvedTerminalLaunch {
  shell?: string
  workingDirectory: string
  initialCommand?: string
  name: string
  profileId?: string
}

export const getShellProfileId = (shellId: string) => `shell:${shellId}`

export const getBuiltInTerminalProfiles = (shells: Shell[]): TerminalProfile[] => [
  {
    id: SYSTEM_DEFAULT_PROFILE_ID,
    name: DEFAULT_PROFILE_LABEL,
  },
  ...shells.map((shell) => ({
    id: getShellProfileId(shell.id),
    name: shell.name,
    shell: shell.id,
    icon: 'terminal',
  })),
]

export const getAllTerminalProfiles = (
  shells: Shell[],
  customProfiles: TerminalProfile[],
): TerminalProfile[] => [...getBuiltInTerminalProfiles(shells), ...customProfiles]
