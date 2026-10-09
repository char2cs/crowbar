/** What a restored chat pane says once the project list has loaded and still does not
 *  place the chat's project: the wait is over, and the tab is the user's to close. */
export function AgentChatUnavailable() {
  return (
    <div className="flex h-full w-full flex-col items-center justify-center gap-1 p-8 text-center">
      <span className="text-sm font-medium text-foreground">Chat unavailable</span>
      <span className="text-xs text-muted-foreground">
        The project or host this chat belongs to is not available. Close this tab, or pair the host
        again to reload it.
      </span>
    </div>
  )
}
