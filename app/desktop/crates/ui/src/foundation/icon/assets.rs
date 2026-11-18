//! Asset source serving the embedded icons.

use std::borrow::Cow;

use gpui::{AssetSource, SharedString};

use crate::foundation::icon::file::File;
use crate::foundation::icon::files::FILES;

pub(crate) const WORDMARK_PATH: &str = "brand/crowbar-wordmark.svg";
const WORDMARK: &[u8] = include_bytes!("../../../assets/brand/crowbar-wordmark.svg");

/// The [`AssetSource`] for `icons/<weight>/<stem>.svg` and the Crowbar wordmark; pass it to `Application::with_assets`.
pub fn assets() -> impl AssetSource {
    Assets { files: FILES }
}

struct Assets {
    files: &'static [File],
}

impl AssetSource for Assets {
    fn load(
        &self,
        path: &str,
    ) -> gpui::Result<Option<Cow<'static, [u8]>>> {
        if path == WORDMARK_PATH {
            return Ok(Some(Cow::Borrowed(WORDMARK)));
        }
        let found = self.files.iter().find(|file| file.path == path);
        Ok(found.map(|file| Cow::Borrowed(file.bytes)))
    }

    fn list(
        &self,
        path: &str,
    ) -> gpui::Result<Vec<SharedString>> {
        Ok(self
            .files
            .iter()
            .filter(|file| file.path.starts_with(path))
            .map(|file| SharedString::from(file.path))
            .collect())
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeSet;
    use std::path::Path;

    use super::*;

    #[ignore = "real-infra: compares the embedded icon table with the real assets/icons directory"]
    fn files_of(weight: &Path) -> Vec<String> {
        let folder = weight
            .file_name()
            .expect("folder")
            .to_string_lossy()
            .into_owned();
        std::fs::read_dir(weight)
            .expect("weight dir")
            .map(|file| {
                file.expect("entry")
                    .file_name()
                    .into_string()
                    .expect("utf8")
            })
            .map(|name| format!("icons/{folder}/{name}"))
            .collect()
    }

    #[test]
    fn load_returns_embedded_svg_and_none_for_unknown() {
        const CASES: &[(&str, bool)] = &[
            ("icons/regular/gear.svg", true),
            ("icons/regular/folder-plus.svg", true),
            ("icons/regular/caret-double-up.svg", true),
            ("icons/regular/image.svg", true),
            ("icons/regular/link.svg", true),
            ("icons/regular/file-text.svg", true),
            ("icons/regular/folder-simple-plus.svg", true),
            ("icons/regular/folder-minus.svg", true),
            ("icons/regular/scissors.svg", true),
            ("icons/regular/copy-simple.svg", true),
            ("icons/regular/nope.svg", false),
            ("brand/crowbar-wordmark.svg", true),
        ];
        for &(path, embedded) in CASES {
            let found = assets().load(path).expect("load");
            assert_eq!(found.is_some(), embedded, "{path}");
            assert!(found.is_none_or(|svg| svg.starts_with(b"<svg")), "{path}");
        }
    }

    #[test]
    fn list_filters_by_prefix() {
        let bold = assets().list("icons/bold/").expect("list");
        assert!(bold.len() > 5, "the bold weight ships icons");
        assert!(bold.iter().all(|name| name.starts_with("icons/bold/")));
    }

    #[test]
    #[ignore = "real-infra: compares the embedded icon table with the real assets/icons directory"]
    fn every_icon_name_has_a_regular_or_other_weight_and_no_file_is_orphaned() {
        let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("assets/icons");
        let on_disk: BTreeSet<String> = std::fs::read_dir(&root)
            .expect("icons dir")
            .flat_map(|weight| files_of(&weight.expect("entry").path()))
            .collect();
        let embedded: BTreeSet<String> = FILES.iter().map(|file| file.path.to_owned()).collect();
        assert_eq!(embedded, on_disk, "embedded table and assets/icons differ");
        let stems: BTreeSet<&str> = crate::foundation::icon::icon_name::IconName::ALL
            .iter()
            .map(|icon| icon.stem())
            .collect();
        for stem in stems {
            let found = embedded
                .iter()
                .any(|name| name.ends_with(&format!("/{stem}.svg")));
            assert!(found, "icon {stem} has no embedded weight");
        }
    }
}
