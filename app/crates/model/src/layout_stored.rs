use std::borrow::Cow;

use protocol::{PaneId, PaneTypeId, Params};
use serde::{Deserialize, Deserializer, Serialize, Serializer};
use serde_json::Value;

use crate::error::{Error, Result};
use crate::layout::{Layout, MAX_TABS};
use crate::layout_document::LayoutDocument;
use crate::schema::{SCHEMA_VERSION, upgrade};
use crate::tab::Tab;
use crate::tab_id::TabId;

impl Layout {
    /// Checks every invariant.
    ///
    /// # Errors
    /// [`Error::Invalid`] naming the first broken invariant.
    pub fn validate(&self) -> Result<()> {
        if self.tabs.len() > MAX_TABS {
            return Err(Error::Invalid("too many tabs"));
        }
        for tab in &self.tabs {
            tab.validate()?;
        }
        let mut panes: Vec<PaneId> = self.tabs.iter().flat_map(Tab::pane_ids).collect();
        if has_duplicates(&mut panes) {
            return Err(Error::Invalid("a pane appears twice"));
        }
        let mut ids: Vec<TabId> = self.tabs.iter().map(Tab::id).collect();
        if has_duplicates(&mut ids) {
            return Err(Error::Invalid("a tab id appears twice"));
        }
        let mut identities: Vec<(&PaneTypeId, &Params)> = self
            .tabs
            .iter()
            .flat_map(Tab::instances)
            .filter(|instance| !instance.params.is_empty())
            .map(|instance| (&instance.type_id, &instance.params))
            .collect();
        if has_duplicates(&mut identities) {
            return Err(Error::Invalid("a pane is open twice"));
        }
        self.validate_active()
    }

    fn validate_active(&self) -> Result<()> {
        let active_ok = match self.active {
            Some(id) => self.tab(id).is_some(),
            None => self.tabs.is_empty(),
        };
        active_ok
            .then_some(())
            .ok_or(Error::Invalid("active tab does not match the tabs"))
    }
}

fn has_duplicates<T: Ord>(items: &mut [T]) -> bool {
    items.sort();
    items.windows(2).any(|pair| pair.first() == pair.get(1))
}

impl Serialize for Layout {
    fn serialize<S: Serializer>(
        &self,
        serializer: S,
    ) -> std::result::Result<S::Ok, S::Error> {
        LayoutDocument {
            version: SCHEMA_VERSION,
            tabs: Cow::Borrowed(&self.tabs),
            active: self.active,
        }
        .serialize(serializer)
    }
}

impl<'de> Deserialize<'de> for Layout {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> std::result::Result<Self, D::Error> {
        let stored = Value::deserialize(deserializer)?;
        Self::from_stored(stored).map_err(serde::de::Error::custom)
    }
}

impl Layout {
    pub(crate) fn from_stored(stored: Value) -> Result<Self> {
        let upgraded = upgrade(stored)?;
        let document: LayoutDocument<'static> = serde_json::from_value(upgraded)
            .map_err(|source| Error::Unreadable(source.to_string()))?;
        let layout = Self {
            tabs: document.tabs.into_owned(),
            active: document.active,
        };
        layout.validate()?;
        Ok(layout)
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::split_node::SplitNode;
    use crate::tab_shape::TabShape;
    use crate::test_support::layout_fixtures::{chat, sample};
    use crate::test_support::tab_fixtures::id;

    #[test]
    fn validate_names_broken_invariants() {
        let mut duplicated = sample();
        duplicated.tabs.push(duplicated.tabs[0].clone());
        let mut same_chat = sample();
        same_chat.tabs[1] = Tab::restored(
            TabShape {
                id: TabId(2),
                title: "two".into(),
                tree: Some(SplitNode::leaf(id("d"))),
                focused: Some(id("d")),
            },
            [(id("d"), chat("d", "c1"))].into_iter().collect(),
        );
        let cases = [
            (
                "active tab missing",
                Layout {
                    active: Some(TabId(9)),
                    ..sample()
                },
            ),
            (
                "tabs but no active",
                Layout {
                    active: None,
                    ..sample()
                },
            ),
            ("tab id and panes twice", duplicated),
            ("one chat open twice", same_chat),
        ];
        for (name, broken) in cases {
            assert!(broken.validate().is_err(), "{name}");
        }
    }

    #[test]
    fn a_layout_round_trips_through_json_with_its_version() {
        let layout = sample();
        let value = serde_json::to_value(&layout).unwrap();
        assert_eq!(value["version"], json!(SCHEMA_VERSION), "versioned");
        let back: Layout = serde_json::from_value(value).unwrap();
        assert_eq!(back, layout, "round trip");
    }

    #[test]
    fn an_unversioned_layout_from_the_first_format_is_migrated() {
        let mut value = serde_json::to_value(sample()).unwrap();
        value.as_object_mut().unwrap().remove("version");
        let migrated: Layout = serde_json::from_value(value).unwrap();
        assert_eq!(migrated, sample(), "same layout");
    }

    #[test]
    fn a_layout_from_a_newer_app_or_with_stray_data_is_refused() {
        let value = serde_json::to_value(sample()).unwrap();
        let cases = [
            ("newer", "version", json!(SCHEMA_VERSION + 1), "newer"),
            ("unknown field", "surprise", json!(true), ""),
            ("dangling active tab", "active", json!(77), "invalid layout"),
        ];
        for (name, field, bad, expected) in cases {
            let mut document = value.clone();
            document[field] = bad;
            let error = serde_json::from_value::<Layout>(document)
                .unwrap_err()
                .to_string();
            assert!(error.contains(expected), "{name}: {error}");
        }
    }
}
