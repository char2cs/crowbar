use ideshell::TabLook;
use layout::PaneInstance;
use sidebar::FileKind;
use sidebarview::FileGlyph;

use super::{FileIcons, ShellPaneSource};

impl ShellPaneSource {
    pub(crate) fn iconing(
        mut self,
        theme: FileIcons,
    ) -> Self {
        self.icons.0 = theme;
        self
    }

    pub(super) fn file_icon(
        &self,
        instance: &PaneInstance,
        look: TabLook,
    ) -> TabLook {
        if !crate::tab_looks::is_file(instance) {
            return look;
        }
        match FileGlyph::of(
            ((self.icons.0)(), FileKind::File),
            &look.label,
            &self.icons.1,
        ) {
            FileGlyph::Phosphor(icon) => TabLook { icon, ..look },
            FileGlyph::Monochrome(svg) => look.drawn(svg),
            FileGlyph::Colored(svg) => look.drawn(svg),
        }
    }
}

#[cfg(test)]
mod tests {
    use ideshell::TabLooks as _;
    use protocol::PaneId;
    use sidebar::IconTheme;
    use ui::{IconName, MaterialTable};

    use super::*;
    use crate::browser_bars::BrowserBars;
    use crate::native_panes::NativePanes;
    use crate::new_tab_panes::NewTabPanes;
    use crate::serials::Serials;
    use crate::terminal_ids::TerminalIds;
    use crate::test_support::idle_terminals;
    use crate::titles::Titles;

    fn pane(
        kind: &str,
        state: &str,
    ) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new("p1").unwrap(),
            type_id: protocol::PaneTypeId::new(kind).unwrap(),
            params: protocol::Params::default(),
            state: protocol::PaneState::new(serde_json::from_str(state).unwrap()).unwrap(),
        }
    }

    fn drawn_by(theme: IconTheme) -> ShellPaneSource {
        ShellPaneSource::new(
            Titles::default(),
            Serials::above(&layout::Layout::default()),
            BrowserBars::new(
                crate::browser_ids::BrowserIds::default(),
                async_channel::unbounded().0,
            ),
            NativePanes::new(
                idle_terminals(TerminalIds::default()),
                NewTabPanes::new(async_channel::unbounded().0),
                crate::test_support::idle_editors(),
            ),
        )
        .iconing(std::rc::Rc::new(move || theme))
    }

    const TEST_FILE: &str = r#"{"path": "src/pricing.test.ts"}"#;
    const EDITOR: &str = "core.editor";

    enum Drawn {
        Icon(IconName),
        Monochrome(&'static str),
        Colored(&'static str),
    }

    impl Drawn {
        fn look(&self) -> (IconName, Option<String>) {
            match self {
                Self::Icon(icon) => (*icon, None),
                Self::Monochrome(name) => (
                    IconName::FileText,
                    Some(MaterialTable::default().icon_of(name).monochrome()),
                ),
                Self::Colored(name) => (
                    IconName::FileText,
                    Some(MaterialTable::default().icon_of(name).colored().to_owned()),
                ),
            }
        }
    }

    type Case = (&'static str, IconTheme, &'static str, &'static str, Drawn);

    const CASES: [Case; 6] = [
        (
            "material",
            IconTheme::Material,
            EDITOR,
            TEST_FILE,
            Drawn::Monochrome("pricing.test.ts"),
        ),
        (
            "colorful",
            IconTheme::ColorfulMaterial,
            EDITOR,
            TEST_FILE,
            Drawn::Colored("pricing.test.ts"),
        ),
        (
            "untitled",
            IconTheme::Material,
            EDITOR,
            r#"{"untitled": true}"#,
            Drawn::Monochrome("Untitled"),
        ),
        (
            "classic",
            IconTheme::Classic,
            EDITOR,
            TEST_FILE,
            Drawn::Icon(IconName::FileText),
        ),
        (
            "compact",
            IconTheme::Compact,
            EDITOR,
            TEST_FILE,
            Drawn::Icon(IconName::File),
        ),
        (
            "a terminal",
            IconTheme::Material,
            "core.terminal",
            "null",
            Drawn::Icon(IconName::TerminalWindow),
        ),
    ];

    #[test]
    fn a_file_tab_draws_the_glyph_its_icon_theme_draws_in_the_files_tree() {
        for (name, theme, kind, state, drawn) in &CASES {
            let look = drawn_by(*theme).tab_look(&pane(kind, state));
            let artwork = look.artwork.map(|svg| svg.to_string());
            assert_eq!((look.icon, artwork), drawn.look(), "{name}");
        }
    }
}
