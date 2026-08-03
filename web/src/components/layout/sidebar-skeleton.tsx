import { Skeleton } from '@/components/ui/skeleton'

export function SidebarSkeleton() {
  return (
    <div className="py-1 px-2 space-y-0.5" data-oracle-id="sidebar-skeleton">
      {/* Chat rows */}
      {[1, 2].map((i) => (
        <div key={i} className="flex h-9 items-center gap-2 mx-1.5 px-2">
          <Skeleton
            className="h-5 w-5 rounded-md flex-shrink-0"
            data-oracle-id={`sidebar-skeleton-chat-${i - 1}-icon`}
          />
          <Skeleton
            className="h-3 flex-1 rounded"
            data-oracle-id={`sidebar-skeleton-chat-${i - 1}-title`}
          />
          <Skeleton
            className="h-3 w-8 rounded"
            data-oracle-id={`sidebar-skeleton-chat-${i - 1}-meta`}
          />
        </div>
      ))}
      <div className="my-1 mx-3 h-px bg-border" data-oracle-id="sidebar-skeleton-divider" />
      {/* Repo + workspace rows */}
      {[1, 2].map((i) => (
        <div key={i} className="space-y-0.5">
          <div className="flex h-9 items-center gap-2 mx-1.5 px-2">
            <Skeleton
              className="h-5 w-5 rounded-md flex-shrink-0"
              data-oracle-id={`sidebar-skeleton-repo-${i - 1}-icon`}
            />
            <Skeleton
              className="h-3 w-24 rounded"
              data-oracle-id={`sidebar-skeleton-repo-${i - 1}-name`}
            />
          </div>
          {[1, 2].map((j) => (
            <div key={j} className="flex h-9 items-center gap-2 mx-1.5 px-2 pl-6">
              <Skeleton
                className="h-3 flex-1 rounded"
                data-oracle-id={`sidebar-skeleton-repo-${i - 1}-workspace-${j - 1}-title`}
              />
              <Skeleton
                className="h-3 w-12 rounded"
                data-oracle-id={`sidebar-skeleton-repo-${i - 1}-workspace-${j - 1}-meta`}
              />
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}
