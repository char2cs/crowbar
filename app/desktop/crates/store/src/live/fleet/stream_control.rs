//! What the shell can ask of a running stream.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum StreamControl {
    /// Ask the daemon for the projects snapshot again, as Retry on a failed load does.
    Resubscribe,
    /// Hold the `files:` scope of the followed chat's workspace, releasing the one held before.
    FollowFiles,
}
