//! Asset source serving the embedded icons.

use std::borrow::Cow;

use gpui::{AssetSource, SharedString};

use crate::icon_files::FILES;

/// [`AssetSource`] for `icons/<weight>/<stem>.svg`; pass it to `Application::with_assets`.
#[derive(Debug, Clone, Copy, Default)]
pub struct Assets;

impl AssetSource for Assets {
    fn load(&self, path: &str) -> gpui::Result<Option<Cow<'static, [u8]>>> {
        let found = FILES.iter().find(|file| file.path == path);
        Ok(found.map(|file| Cow::Borrowed(file.bytes)))
    }

    fn list(&self, path: &str) -> gpui::Result<Vec<SharedString>> {
        Ok(FILES
            .iter()
            .filter(|file| file.path.starts_with(path))
            .map(|file| SharedString::from(file.path))
            .collect())
    }
}

#[cfg(test)]
#[path = "assets_tests.rs"]
mod tests;
