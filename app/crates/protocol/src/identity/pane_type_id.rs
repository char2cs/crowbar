use std::fmt;
use std::str::FromStr;

use serde::{Deserialize, Serialize};

use crate::error::{Error, Result};
use crate::identity::extension_id::ExtensionId;
use crate::identity::pane_kind::PaneKind;
use crate::identity::pane_origin::PaneOrigin;
use crate::string_id::check_text;

mod chat;

fn malformed() -> Error {
    Error::malformed(
        "pane type id",
        "must be `core.<name>` or `ext.<extension>.<name>`",
    )
}

fn parse_origin(value: &str) -> Result<PaneOrigin> {
    check_text("pane type id", value, |c| {
        c.is_ascii_lowercase() || c.is_ascii_digit() || matches!(c, '-' | '_' | '.')
    })?;
    let segments: Vec<&str> = value.split('.').collect();
    if segments.len() < 2 || segments.iter().any(|segment| segment.is_empty()) {
        return Err(malformed());
    }
    match segments.as_slice() {
        ["core", ..] => Ok(PaneOrigin::Core),
        ["ext", extension, _, ..] => Ok(PaneOrigin::Extension(ExtensionId::new(*extension)?)),
        _ => Err(malformed()),
    }
}

/// Namespaced pane type: `core.chat` or `ext.<extension>.<pane>`.
///
/// The owner is parsed once at construction, so [`PaneTypeId::origin`] cannot disagree with the
/// text: an `ext.` type always names a valid [`ExtensionId`].
#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(try_from = "String", into = "String")]
pub struct PaneTypeId {
    text: String,
    origin: PaneOrigin,
}

impl PaneTypeId {
    /// Validates and wraps `value`.
    ///
    /// # Errors
    /// An [`Error`] when `value` is not `core.<name>` or `ext.<extension>.<name>` with a valid
    /// extension id.
    pub fn new(value: impl Into<String>) -> Result<Self> {
        let text = value.into();
        let origin = parse_origin(&text)?;
        Ok(Self { text, origin })
    }

    /// The type as a string slice.
    #[must_use]
    pub fn as_str(&self) -> &str {
        &self.text
    }

    /// Who owns the type.
    #[must_use]
    pub fn origin(&self) -> &PaneOrigin {
        &self.origin
    }

    /// Whether the type belongs to the core app or to an extension.
    #[must_use]
    pub fn kind(&self) -> PaneKind {
        match self.origin {
            PaneOrigin::Core => PaneKind::Core,
            PaneOrigin::Extension(_) => PaneKind::Extension,
        }
    }

    /// The extension that owns the type, or `None` for core types.
    #[must_use]
    pub fn extension(&self) -> Option<ExtensionId> {
        self.origin.extension().cloned()
    }
}

impl TryFrom<String> for PaneTypeId {
    type Error = Error;

    fn try_from(value: String) -> Result<Self> {
        Self::new(value)
    }
}

impl From<PaneTypeId> for String {
    fn from(id: PaneTypeId) -> Self {
        id.text
    }
}

impl FromStr for PaneTypeId {
    type Err = Error;

    fn from_str(value: &str) -> Result<Self> {
        Self::new(value)
    }
}

impl fmt::Display for PaneTypeId {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        f.write_str(&self.text)
    }
}

impl AsRef<str> for PaneTypeId {
    fn as_ref(&self) -> &str {
        &self.text
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, bool); 14] = [
        ("core.chat", true),
        ("ext.git.review", true),
        ("core.a.b", true),
        ("ext.my-ext.x_y", true),
        ("chat", false),
        ("ext.git", false),
        ("other.thing", false),
        ("core..x", false),
        ("core.", false),
        (".core.x", false),
        ("Core.chat", false),
        ("ext.my_ext.bar", false),
        ("ext.1foo.bar", false),
        ("ext.-x.bar", false),
    ];

    #[test]
    fn requires_a_namespace_and_a_valid_extension_segment() {
        for (text, valid) in CASES {
            assert_eq!(PaneTypeId::new(text).is_ok(), valid, "{text}");
        }
    }

    #[test]
    fn knows_its_owner() {
        let core = PaneTypeId::new("core.terminal").unwrap();
        assert_eq!(core.kind(), PaneKind::Core, "core kind");
        assert_eq!(core.extension(), None, "core has no extension");
        assert_eq!(core.origin(), &PaneOrigin::Core, "core origin");
        let ext = PaneTypeId::new("ext.git.review").unwrap();
        assert_eq!(ext.kind(), PaneKind::Extension, "extension kind");
        let git = ExtensionId::new("git").unwrap();
        assert_eq!(ext.extension(), Some(git.clone()), "owner");
        assert_eq!(
            ext.origin(),
            &PaneOrigin::Extension(git),
            "extension origin"
        );
    }

    #[test]
    fn text_and_serde_round_trip_and_validate() {
        let id: PaneTypeId = "ext.git.review".parse().unwrap();
        assert_eq!(id.to_string(), "ext.git.review", "display");
        assert_eq!(id.as_str(), "ext.git.review", "as_str");
        assert_eq!(id.as_ref(), "ext.git.review", "as_ref");
        let json = serde_json::to_string(&id).unwrap();
        assert_eq!(json, "\"ext.git.review\"", "serialize");
        assert_eq!(
            serde_json::from_str::<PaneTypeId>(&json).unwrap(),
            id,
            "round trip"
        );
        assert!(
            serde_json::from_str::<PaneTypeId>("\"ext.my_ext.x\"").is_err(),
            "validates"
        );
        assert_eq!(String::from(id), "ext.git.review", "into string");
    }
}
