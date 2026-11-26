//! The Agents page.

use super::rows::row;
use crate::schema::Action;
use crate::schema::ButtonFace;
use crate::schema::ButtonSpec;
use crate::schema::Control;
use crate::schema::ControlWidth;
use crate::schema::Heading;
use crate::schema::ListBlock;
use crate::schema::Page;
use crate::schema::Row;
use crate::schema::Section;
use crate::values::Value;

pub(super) const AGENTS: &str = "agents";
pub(super) const DESCRIPTORS: &str = "descriptors";

const TITLE: &str = "Agents";
const INTRO: &str = "Agents are the coding CLIs Crowbar runs your chats on — Claude Code, Codex, and any others you add. Crowbar starts one for you when you open a chat, and each keeps its own session and login.";
const REORDER_HINT: &str = "Drag to set which agent a new chat opens first, and turn one off to hide it from every New chat surface. Tools lets that agent use Crowbar itself — reading your workspaces and leaving review comments; turn it off and the agent still runs, with no way in.";

pub(super) fn page() -> Page {
    Page::new("providers", TITLE, "robot")
        .indexed(super::search_index::entries(super::search_index::AGENTS))
        .block(Heading::new(TITLE, INTRO))
        .block(Section::new(TITLE).headerless().row(permission()))
        .block(upload())
        .block(
            ListBlock::new(AGENTS, "No agents available.")
                .intro(REORDER_HINT)
                .states("Loading agents…", "Crowbar could not load the agent list — it could not reach the daemon."),
        )
        .block(
            ListBlock::new(DESCRIPTORS, "No descriptors.")
                .titled("Descriptor checks")
                .states("Checking descriptors…", "Crowbar could not check the descriptors — it could not reach the daemon."),
        )
        .block(Section::new("Chat").row(row(
            ("chatIsDefaultPresentation", "Chat By Default"),
            "Open a chat on Chat rather than Terminal. Both surfaces stay in every chat either way — this only picks the one you land on.",
            Control::switch(),
            Value::Bool(true),
        )))
}

fn permission() -> Row {
    let levels = Control::select(
        &[
            ("guarded", "Guarded"),
            ("trusted", "Trusted"),
            ("full-auto", "Full Auto"),
        ],
        ControlWidth::Default,
    );
    Row::new("defaultPermissionLevel", "Default permission level", levels)
        .describe("How much of a new chat's tool-call approval is answered automatically.")
}

fn upload() -> ButtonSpec {
    ButtonSpec::new(
        "Upload agent descriptor",
        Action::backend("agents.uploadDescriptor"),
    )
    .faced(ButtonFace::Label)
    .noted(
        "Upload a YAML descriptor with a unique provider ID. Its CLI must be installed separately.",
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_daemon_owned_permission_level_offers_no_reset() {
        let schema = crate::default_schema().unwrap();
        let mut state = crate::State::new(
            schema,
            crate::Snapshot::default(),
            None,
            chord::Primary::Control,
        );
        let level = permission();
        drop(state.choose(&level, 2));
        state.acknowledged(level.key());
        assert!(!state.resettable(&level));
    }
}
