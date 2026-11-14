//! Everything the chats panel of one space draws.

use crate::notice::Notice;
use crate::reach::Reach;
use crate::row::Row;

/// The rows of a space, a load failure to show, and whether its host answers.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct ProjectTree {
    /// Rows in tree order.
    pub rows: Vec<Row>,
    /// A failed load to report above the rows.
    pub notice: Option<Notice>,
    /// Reachability of the space's host.
    pub reach: Reach,
}
