//! The Appearance page.

use super::rows::number_setting::NumberSetting;
use super::rows::row;
use crate::schema::Control;
use crate::schema::ControlWidth;
use crate::schema::Page;
use crate::schema::Section;
use crate::values::Value;

const UI_FONT_SIZE: NumberSetting = NumberSetting::normalized((10.0, 24.0, 0.5), 15.0);

pub(super) fn page() -> Page {
    Page::new("appearance", "Appearance", "paint-brush")
        .indexed(super::search_index::entries(
            super::search_index::APPEARANCE,
        ))
        .block(theme())
        .block(icons())
        .block(typography())
        .block(layout())
}

fn theme() -> Section {
    Section::new("Theme")
        .headerless()
        .row(row(
            ("theme", "Color Theme"),
            "Choose your active color theme",
            Control::provided_select("themes", ControlWidth::Wide),
            "crowbar",
        ))
        .row(row(
            ("themeMode", "Theme Mode"),
            "Use light, dark, or follow system preference",
            Control::select(
                &[
                    ("system", "Sync with System"),
                    ("light", "Light"),
                    ("dark", "Dark"),
                ],
                ControlWidth::Wide,
            ),
            "system",
        ))
}

fn icons() -> Section {
    Section::new("Icons").row(row(
        ("iconTheme", "Icon Theme"),
        "Icons displayed in the file tree and tabs",
        Control::select(
            &[
                ("classic", "Classic"),
                ("material", "Material Icons"),
                ("colorful-material", "Colorful Material Icons"),
                ("compact", "Compact"),
                ("minimal", "Minimal Icons"),
                ("none", "None"),
            ],
            ControlWidth::Wide,
        ),
        "material",
    ))
}

fn typography() -> Section {
    Section::new("Typography")
        .row(row(
            ("uiFontFamily", "UI Font Family"),
            "Font family for UI elements (file tree, markdown, etc.)",
            Control::provided_select("fonts", ControlWidth::Text),
            "CalSansUI",
        ))
        .row(row(
            ("headingFontFamily", "Heading Font Family"),
            "Font for headings — Markdown titles, dialog titles, settings section titles",
            Control::provided_select("fonts", ControlWidth::Text),
            "Instrument Serif",
        ))
        .row(row(
            ("uiFontSize", "UI Font Size"),
            "Adjust UI text and icon scale in 0.5px steps",
            UI_FONT_SIZE.snapped(ControlWidth::Compact),
            UI_FONT_SIZE.default,
        ))
}

fn layout() -> Section {
    Section::new("Layout")
        .row(row(
            ("sidebarPosition", "Sidebar Position"),
            "Which side the file sidebar appears on",
            Control::select(&[("left", "Left"), ("right", "Right")], ControlWidth::Compact),
            "left",
        ))
        .row(row(
            ("tabListScope", "Tab list"),
            "Which tabs the strip lists: those of the current project, or every tab open on this client across all projects and hosts",
            Control::select(
                &[("project", "Current project"), ("all", "Everything on this client")],
                ControlWidth::Compact,
            ),
            "project",
        ))
        .row(row(
            ("sidebarFollowsFocus", "Sidebar follows focus"),
            "Switch the sidebar between Chats, Files and Git as focus moves between chats, editors and diffs.",
            Control::switch(),
            Value::Bool(false),
        ))
        .row(row(
            ("buildReadout", "Build readout"),
            "Show the build, daemon connection, frame rate and open terminal, agent and workspace counts at the foot of the sidebar, or in the tab strip while it is hidden",
            Control::switch(),
            Value::Bool(false),
        ))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rows_follow_the_spec_order_and_only_the_first_section_hides_its_header() {
        let page = page();
        let keys: Vec<&str> = page.rows().map(|row| row.key().as_str()).collect();
        assert_eq!(
            keys,
            [
                "theme",
                "themeMode",
                "iconTheme",
                "uiFontFamily",
                "headingFontFamily",
                "uiFontSize",
                "sidebarPosition",
                "tabListScope",
                "sidebarFollowsFocus",
                "buildReadout",
            ]
        );
        let headers: Vec<bool> = page
            .blocks
            .iter()
            .filter_map(|block| match block {
                crate::schema::Block::Section(section) => Some(section.show_header),
                _ => None,
            })
            .collect();
        assert_eq!(headers, [false, true, true, true]);
    }

    #[test]
    fn icon_themes_are_offered_in_registration_order() {
        let page = page();
        let Some(icon) = page.rows().find(|row| row.key().as_str() == "iconTheme") else {
            panic!("no icon theme row");
        };
        let Control::Select {
            source: crate::schema::ChoiceSource::Fixed(choices),
            ..
        } = icon.control()
        else {
            panic!("icon theme is not a fixed select");
        };
        let labels: Vec<&str> = choices.iter().map(crate::values::Choice::label).collect();
        assert_eq!(
            labels,
            [
                "Classic",
                "Material Icons",
                "Colorful Material Icons",
                "Compact",
                "Minimal Icons",
                "None"
            ]
        );
    }
}
