//! The Appearance page.

use super::rows::{row, snapped};
use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::page::Page;
use crate::section::Section;

pub(super) fn page() -> Page {
    Page::new("appearance", "Appearance", "paint-brush")
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
                ("material", "Material Icons"),
                ("colorful-material", "Colorful Material Icons"),
                ("classic", "Classic"),
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
            "Font for headings - Markdown titles, dialog titles, settings section titles",
            Control::provided_select("fonts", ControlWidth::Text),
            "Instrument Serif",
        ))
        .row(row(
            ("uiFontSize", "UI Font Size"),
            "Adjust UI text and icon scale in 0.5px steps",
            snapped((10.0, 24.0, 0.5), ControlWidth::Compact),
            15.0,
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
            false,
        ))
        .row(row(
            ("buildReadout", "Build readout"),
            "Show the build, daemon connection, frame rate and open terminal, agent and workspace counts at the foot of the sidebar, or in the tab strip while it is hidden",
            Control::switch(),
            false,
        ))
}

#[cfg(test)]
#[path = "appearance_tests.rs"]
mod tests;
