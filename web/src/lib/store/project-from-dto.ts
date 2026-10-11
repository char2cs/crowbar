import type { Project, ProjectDTO } from '@/lib/types'

// Map the §5 ProjectDTO (lastActivity as an ISO string) onto the UI `Project`
// shape the project cards/store consume (lastActivity as a Date). A missing or
// unparseable timestamp falls back to now so the card never shows "Invalid Date".
// `hostId` is the daemon the DTO was read from — the wire never names it.
export function projectFromDTO(dto: ProjectDTO, hostId: string): Project {
  const parsed = dto.lastActivity ? new Date(dto.lastActivity) : new Date()
  return {
    id: dto.id,
    name: dto.name,
    path: dto.path,
    lastActivity: Number.isNaN(parsed.getTime()) ? new Date() : parsed,
    hostId,
  }
}
