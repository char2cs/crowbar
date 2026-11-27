use crate::records::RecordCommand;
use crate::values::Key;
use crate::values::Value;

#[derive(Debug, Clone, PartialEq)]
pub enum Effect {
    Write(Key, Value),
    Invoke(String),
    Record {
        source: String,
        record: String,
        command: RecordCommand,
    },
}
