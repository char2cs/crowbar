//! The modal's access to record lists.

use crate::records::Records;
use crate::records::record_command::RecordCommand;
use crate::values::Error;

/// Where agents, active sessions and hosts come from.
pub trait RecordBackend {
    /// The current content of the list `source`.
    fn records(
        &self,
        source: &str,
    ) -> Records;

    /// Applies `command` to the record `record` of the list `source`.
    ///
    /// # Errors
    /// [`Error::Unavailable`] when the owner of the list cannot be reached.
    fn apply(
        &self,
        source: &str,
        record: &str,
        command: RecordCommand,
    ) -> Result<(), Error>;
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use super::*;
    use crate::records::fake_backend::FakeBackend;
    use crate::records::record::Record;

    #[test]
    fn a_backend_is_usable_behind_the_trait_object() {
        let backend: Rc<dyn RecordBackend> = Rc::new(
            FakeBackend::default()
                .with_records("agents", Records::Ready(vec![Record::new("a", "A")])),
        );
        assert!(matches!(backend.records("agents"), Records::Ready(list) if list.len() == 1));
        assert_eq!(backend.records("elsewhere"), Records::Loading);
        backend
            .apply("agents", "a", RecordCommand::Refresh)
            .unwrap();
    }
}
