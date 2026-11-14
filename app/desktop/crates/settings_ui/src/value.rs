//! The value of a setting.

use std::collections::BTreeMap;

/// A setting value; the schema decides which shape each key holds.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum Value {
    /// On or off.
    Bool(bool),
    /// Free text or an option value.
    Text(String),
    /// A number.
    Number(f64),
    /// An ordered list of strings.
    List(Vec<String>),
    /// String pairs, such as command to chord.
    Map(BTreeMap<String, String>),
}

impl Value {
    /// The flag, when this is a [`Value::Bool`].
    pub fn as_bool(&self) -> Option<bool> {
        match self {
            Self::Bool(flag) => Some(*flag),
            _ => None,
        }
    }

    /// The text, when this is a [`Value::Text`].
    pub fn as_text(&self) -> Option<&str> {
        match self {
            Self::Text(text) => Some(text),
            _ => None,
        }
    }

    /// The number, when this is a [`Value::Number`].
    pub fn as_number(&self) -> Option<f64> {
        match self {
            Self::Number(number) => Some(*number),
            _ => None,
        }
    }

    /// The items, when this is a [`Value::List`].
    pub fn as_list(&self) -> Option<&[String]> {
        match self {
            Self::List(items) => Some(items),
            _ => None,
        }
    }

    /// The pairs, when this is a [`Value::Map`].
    pub fn as_map(&self) -> Option<&BTreeMap<String, String>> {
        match self {
            Self::Map(pairs) => Some(pairs),
            _ => None,
        }
    }
}

impl From<bool> for Value {
    fn from(flag: bool) -> Self {
        Self::Bool(flag)
    }
}

impl From<&str> for Value {
    fn from(text: &str) -> Self {
        Self::Text(text.to_owned())
    }
}

impl From<f64> for Value {
    fn from(number: f64) -> Self {
        Self::Number(number)
    }
}

#[cfg(test)]
#[path = "value_tests.rs"]
mod tests;
