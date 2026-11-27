use crate::records::Records;
use crate::records::record_command::RecordCommand;
use crate::values::Error;

pub trait RecordBackend {
    fn records(
        &self,
        source: &str,
    ) -> Records;

    fn apply(
        &self,
        source: &str,
        record: &str,
        command: RecordCommand,
    ) -> Result<(), Error>;
}
