/// What a view did with the text its file holds on disk.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DiskTake {
    /// Nothing changed: disk holds what the view knows, the read was older than the latest save,
    /// or a change was already flagged.
    Kept,
    /// The view was clean and now shows the disk text as saved.
    Taken,
    /// The view holds edits and disk moved under them: the edits stay and the user is told once.
    Conflicted,
}
