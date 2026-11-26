use std::borrow::Cow;

use protocol::{PaneId, PaneTypeId, Params};
use serde::{Deserialize, Deserializer, Serialize, Serializer};
use serde_json::Value;

use crate::error::{Error, Result};
use crate::layout::stored::layout_document::LayoutDocument;
use crate::layout::stored::schema::{SCHEMA_VERSION, current};
use crate::layout::tab::Tab;
use crate::layout::{Layout, MAX_TABS};
use crate::space_id::SpaceId;
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
        self.validate_shown()
    }

    fn validate_shown(&self) -> Result<()> {
        let own =
            |(space, id): (&SpaceId, &TabId)| self.tab(*id).is_some_and(|tab| &tab.space == space);
        self.shown
            .iter()
            .all(own)
            .then_some(())
            .ok_or(Error::Invalid("a space shows a tab that is not its own"))
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
            shown: Cow::Borrowed(&self.shown),
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
        let Some(stored) = current(stored)? else {
            return Ok(Self::default());
        };
        let document: LayoutDocument<'static> = serde_json::from_value(stored)
            .map_err(|source| Error::Unreadable(source.to_string()))?;
        let layout = Self {
            tabs: document.tabs.into_owned(),
            shown: document.shown.into_owned(),
        };
        layout.validate()?;
        Ok(layout)
    }
}

/// Reads a layout from its stored JSON.
///
/// A document of another schema version, older or newer, loads as the default layout with a
/// warning; the result satisfies every [`Layout`] invariant, so a damaged file is an error rather
/// than a broken window.
///
/// # Errors
/// [`Error::Unreadable`] for text that is not a layout document and [`Error::Invalid`] for data
/// that breaks an invariant.
pub fn load(stored: &[u8]) -> Result<Layout> {
    let value =
        serde_json::from_slice(stored).map_err(|source| Error::Unreadable(source.to_string()))?;
    Layout::from_stored(value)
}

/// Writes a layout as JSON in the current format.
///
/// # Errors
/// [`Error::Unreadable`] when the layout cannot be encoded.
pub fn store(layout: &Layout) -> Result<Vec<u8>> {
    serde_json::to_vec(layout).map_err(|source| Error::Unreadable(source.to_string()))
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::layout::tab::tab_shape::TabShape;
    use crate::test_support::layout_fixtures::{chat, sample};
    use crate::test_support::tab_fixtures::id;
    use crate::tree::split_node::SplitNode;

    #[test]
    fn validate_names_broken_invariants() {
        let mut duplicated = sample();
        duplicated.tabs.push(duplicated.tabs[0].clone());
        let mut same_chat = sample();
        same_chat.tabs[1] = Tab::restored(
            TabShape {
                id: TabId(2),
                title: "two".into(),
                space: SpaceId::default(),
                tree: Some(SplitNode::leaf(id("d"))),
                focused: Some(id("d")),
            },
            [(id("d"), chat("d", "c1"))].into_iter().collect(),
        );
        let cases = [
            (
                "shown tab missing",
                Layout {
                    shown: [(SpaceId::default(), TabId(9))].into_iter().collect(),
                    ..sample()
                },
            ),
            (
                "shown in a space it is not of",
                Layout {
                    shown: [(SpaceId::new("p1"), TabId(1))].into_iter().collect(),
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
    fn a_layout_of_another_version_loads_as_the_default_and_stray_data_is_refused() {
        let value = serde_json::to_value(sample()).unwrap();
        for version in [None, Some(SCHEMA_VERSION - 2), Some(SCHEMA_VERSION + 1)] {
            let mut document = value.clone();
            document["version"] = version.map_or(Value::Null, Value::from);
            let loaded: Layout = serde_json::from_value(document).unwrap();
            assert_eq!(loaded, Layout::default(), "{version:?}");
        }
        let cases = [
            ("surprise", json!(true), "unknown field"),
            ("shown", json!({"": 77}), "invalid layout"),
        ];
        for (field, bad, expected) in cases {
            let mut document = value.clone();
            document[field] = bad;
            let error = serde_json::from_value::<Layout>(document).unwrap_err();
            assert!(error.to_string().contains(expected), "{error}");
        }
    }

    const V3_FIXTURE: &[u8] = include_bytes!("../../../tests/fixtures/layout-v3.json");

    fn assert_kept(
        tab: &Tab,
        record: &Value,
    ) {
        let name = tab.id().0;
        assert_eq!(tab.space(), "", "tab {name} waits for a space");
        assert_eq!(tab.title(), record["title"], "tab {name}: title");
        let panes = serde_json::to_value(&tab.panes).unwrap();
        assert_eq!(panes, record["panes"], "tab {name}: panes and their state");
        let tree = serde_json::to_value(tab.tree()).unwrap();
        assert_eq!(tree, record["tree"], "tab {name}: tree");
        let focused = serde_json::to_value(tab.focused()).unwrap();
        assert_eq!(focused, record["focused"], "tab {name}: focus");
    }

    #[test]
    fn a_v3_layout_loads_with_its_tabs_unowned_and_shown_in_no_space() {
        let stored: Value = serde_json::from_slice(V3_FIXTURE).unwrap();
        let layout = load(V3_FIXTURE).unwrap();
        let ids: Vec<u64> = layout.tabs().iter().map(|tab| tab.id().0).collect();
        assert_eq!(ids, [1, 2, 3], "every tab kept, in order");
        let records = stored["tabs"].as_array().unwrap();
        for (tab, record) in layout.tabs().iter().zip(records) {
            assert_kept(tab, record);
        }
        let shown = layout.showing("").map(Tab::id);
        assert_eq!(shown, Some(TabId(2)), "the old active tab");
        assert!(layout.showing("p1").is_none(), "no project shows it");
        let resaved: Value = serde_json::to_value(&layout).unwrap();
        assert_eq!(
            resaved["version"],
            json!(SCHEMA_VERSION),
            "saved as current"
        );
        assert_eq!(resaved["shown"], json!({"": 2}), "shown in no space");
    }

    #[test]
    fn a_v4_layout_round_trips_its_spaces_and_refuses_a_tab_shown_in_another_space() {
        let mut layout = sample();
        layout.tabs[1].space = SpaceId::new("p2");
        layout.shown = [
            (SpaceId::default(), TabId(1)),
            (SpaceId::new("p2"), TabId(2)),
        ]
        .into_iter()
        .collect();
        let value = serde_json::to_value(&layout).unwrap();
        assert_eq!(value["version"], json!(4), "v4");
        assert_eq!(value["tabs"][1]["space"], json!("p2"), "space stored");
        assert_eq!(value["shown"], json!({"": 1, "p2": 2}), "shown stored");
        let back: Layout = serde_json::from_value(value.clone()).unwrap();
        assert_eq!(back, layout, "round trip");
        let mut crossed = value;
        crossed["shown"] = json!({"p2": 1});
        let error = serde_json::from_value::<Layout>(crossed).unwrap_err();
        assert!(error.to_string().contains("invalid layout"), "{error}");
    }

    #[test]
    fn unreadable_and_broken_data_are_distinct_errors() {
        let broken = json!({"version": SCHEMA_VERSION, "tabs": [], "shown": {"": 4}});
        let shape = json!({"version": SCHEMA_VERSION, "tabs": 3});
        let unreadable = Error::Unreadable(String::new());
        let cases = [
            (b"not json".to_vec(), &unreadable),
            (shape.to_string().into_bytes(), &unreadable),
            (broken.to_string().into_bytes(), &Error::Invalid("")),
        ];
        for (bytes, kind) in cases {
            let error = load(&bytes).unwrap_err();
            let same = std::mem::discriminant(&error) == std::mem::discriminant(kind);
            assert!(same, "{error:?}");
        }
        let empty = Layout::default();
        assert_eq!(load(&store(&empty).unwrap()), Ok(empty), "round trip");
    }
}
