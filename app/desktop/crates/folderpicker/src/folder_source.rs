mod reader;
mod requests;

pub(crate) use reader::Reader;
pub(crate) use requests::Requests;

pub(crate) trait FolderSource: Reader + Requests {}

impl<T: Reader + Requests> FolderSource for T {}

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
