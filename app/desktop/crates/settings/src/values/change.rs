use crate::values::key::Key;
use crate::values::value::Value;

#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum Change {
    Setting(Key, Value),
    Records(String),
}
