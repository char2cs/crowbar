//! Demo backend data for the preview.

use settings_ui::{
    AGENT_RECORDS, Choice, DESCRIPTOR_RECORDS, FakeBackend, HOST_RECORDS, Line, Record,
    RecordAction, RecordFlag, Records, SESSION_RECORDS, Snapshot, Tone, Values,
};

fn choices(names: &[&str]) -> Vec<Choice> {
    names.iter().map(|name| Choice::new(*name, *name)).collect()
}

fn snapshot() -> Snapshot {
    let themes = vec![
        Choice::new("crowbar", "Crowbar"),
        Choice::new("frosted", "Frosted"),
    ];
    Snapshot::new(Values::default())
        .with_capability("browser.clearData")
        .with_choices("themes", themes)
        .with_choices(
            "fonts",
            choices(&["CalSansUI", "Instrument Serif", "Inter", "System UI"]),
        )
        .with_choices(
            "monoFonts",
            choices(&["Geist Mono Variable", "JetBrains Mono", "Fira Code"]),
        )
        .with_choices(
            "terminalFonts",
            choices(&["Geist Mono Variable", "JetBrainsMono Nerd Font", "Menlo"]),
        )
}

const SPARK: &str = "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\"><circle cx=\"12\" cy=\"12\" r=\"10\" fill=\"#d97757\"/></svg>";

fn agents() -> Records {
    let claude = Record::new("claude", "Claude Code")
        .artwork(SPARK)
        .status(Tone::Success)
        .line(Line::plain("Signed in"))
        .line(Line::plain("/usr/bin/claude").mono())
        .flag(RecordFlag::new("connected", "Installed").on().as_dot())
        .flag(RecordFlag::new("enabled", "Enabled").on())
        .flag(RecordFlag::new("tools", "Tools").on())
        .movable();
    let codex = Record::new("codex", "Codex")
        .icon("terminal-window")
        .status(Tone::Warning)
        .line(Line::plain("Not signed in").toned(Tone::Warning))
        .flag(RecordFlag::new("connected", "Installed").as_dot())
        .flag(RecordFlag::new("enabled", "Enabled").on())
        .flag(RecordFlag::new("tools", "Tools"))
        .movable();
    Records::Ready(vec![claude, codex])
}

fn sessions() -> Records {
    let session = Record::new("s1", "Crowbar - migration/rust-shell")
        .icon("terminal-window")
        .status(Tone::Success)
        .line(Line::plain("Claude Code - 12 minutes").mono())
        .action(RecordAction::new("stop", "trash", "Stop session"));
    Records::Ready(vec![session])
}

fn hosts() -> Records {
    let host = Record::new("h1", "build-box")
        .icon("terminal-window")
        .status(Tone::Pending)
        .line(Line::plain("build-box.local:7421").mono())
        .action(RecordAction::new("revoke", "trash", "Revoke"));
    Records::Ready(vec![host])
}

pub(crate) fn backend() -> FakeBackend {
    FakeBackend::new(snapshot())
        .with_records(AGENT_RECORDS, agents())
        .with_records(DESCRIPTOR_RECORDS, Records::Ready(Vec::new()))
        .with_records(SESSION_RECORDS, sessions())
        .with_records(HOST_RECORDS, hosts())
}
