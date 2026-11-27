use crate::values::key::Key;
use crate::values::snapshot::Snapshot;
use crate::values::value::Value;

pub trait Read {
    fn snapshot(&self) -> Snapshot;

    fn get(
        &self,
        key: &Key,
    ) -> Option<Value>;
}
