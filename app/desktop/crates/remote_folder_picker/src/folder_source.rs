//! What the picker needs of the data behind it, so the data can be replaced in tests.

use crate::folder_reader::FolderReader;
use crate::folder_requests::FolderRequests;

pub(crate) trait FolderSource: FolderReader + FolderRequests {}

impl<T: FolderReader + FolderRequests> FolderSource for T {}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use super::*;
    use crate::test_support::reachable_fake;

    #[test]
    fn anything_that_reads_and_requests_is_a_source() {
        let source: Rc<dyn FolderSource> = reachable_fake();
        assert_eq!(source.host_name(), "Box", "reads");
        source.request("/srv");
        source.retry("/srv");
    }
}
