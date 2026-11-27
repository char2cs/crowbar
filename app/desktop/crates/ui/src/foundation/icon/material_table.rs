use gpui::{App, Global};

use crate::foundation::icon::material_entry::MaterialEntry;
use crate::foundation::icon::material_icon::MaterialIcon;
use crate::foundation::opaque_debug;

const TABLE: &str = include_str!("../../../assets/file-icons/material-icons.tsv");
const NO_ICON: MaterialIcon = MaterialIcon { name: "", svg: "" };

pub struct MaterialTable {
    fallback: MaterialIcon,
    entries: Vec<MaterialEntry>,
}

impl Global for MaterialTable {}

opaque_debug!(MaterialTable);

impl Default for MaterialTable {
    fn default() -> Self {
        let mut parsed = TABLE.lines().filter_map(MaterialEntry::parse);
        let fallback = parsed.next().map_or(NO_ICON, |entry| entry.icon);
        let entries = parsed.collect();
        Self { fallback, entries }
    }
}

impl MaterialIcon {
    #[must_use]
    pub fn of(
        file_name: &str,
        cx: &mut App,
    ) -> Self {
        cx.default_global::<MaterialTable>().icon_of(file_name)
    }
}

impl MaterialTable {
    #[must_use]
    pub fn icon_of(
        &self,
        file_name: &str,
    ) -> MaterialIcon {
        self.find(&file_name.to_lowercase())
    }

    pub(crate) fn find(
        &self,
        lowered: &str,
    ) -> MaterialIcon {
        let named = self.entries.iter().find(|entry| entry.names(lowered));
        if let Some(entry) = named {
            return entry.icon;
        }
        self.entries
            .iter()
            .rev()
            .filter_map(|entry| entry.extension_of(lowered).map(|length| (entry, length)))
            .max_by_key(|(_, length)| *length)
            .map_or(self.fallback, |(entry, _)| entry.icon)
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn a_file_name_picks_develops_icon(cx: &mut TestAppContext) {
        const CASES: [(&str, &str); 7] = [
            ("pricing.ts", "typescript"),
            ("logo.PNG", "image"),
            ("README.md", "readme"),
            ("package.json", "nodejs"),
            ("types.d.ts", "typescript-def"),
            ("GNUmakefile", "makefile"),
            ("no-extension", "file"),
        ];
        for (file, expected) in CASES {
            let icon = cx.update(|cx| MaterialIcon::of(file, cx));
            assert_eq!(icon.name(), expected, "{file}");
        }
    }
}
