//! How the settings dress a row of the Files and Changes panels.

use sidebar::{FileTreeView, IconTheme, IndentGuides};

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct TreeLook {
    pub(crate) indent: f32,
    pub(crate) height: f32,
    pub(crate) query: Option<String>,
    pub(crate) compact_badges: bool,
    pub(crate) icons: IconTheme,
    pub(crate) guides: IndentGuides,
}

impl TreeLook {
    pub(crate) fn of(
        view: &FileTreeView,
        query: Option<&str>,
    ) -> Self {
        Self {
            indent: view.indent,
            height: view.density.row_height(),
            query: query
                .map(str::trim)
                .filter(|text| !text.is_empty())
                .map(str::to_owned),
            compact_badges: view.compact_badges,
            icons: view.icons,
            guides: view.guides,
        }
    }
}

impl Default for TreeLook {
    fn default() -> Self {
        Self::of(&FileTreeView::default(), None)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::FileDensity;

    #[test]
    fn the_look_carries_the_settings_and_drops_a_blank_query() {
        let view = FileTreeView {
            indent: 20.0,
            density: FileDensity::Comfortable,
            compact_badges: true,
            ..FileTreeView::default()
        };
        let look = TreeLook::of(&view, Some("  "));
        assert!((look.indent - 20.0).abs() < f32::EPSILON);
        assert!((look.height - 30.0).abs() < f32::EPSILON);
        assert!(look.compact_badges);
        assert_eq!(look.query, None);
        assert_eq!(TreeLook::of(&view, Some(" a ")).query.as_deref(), Some("a"));
    }
}
