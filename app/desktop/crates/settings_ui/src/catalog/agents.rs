//! The Agents page.

use super::rows::row;
use crate::action::Action;
use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::heading::Heading;
use crate::list_block::ListBlock;
use crate::page::Page;
use crate::row::Row;
use crate::section::Section;
use crate::value::Value;

pub(super) const AGENTS: &str = "agents";
pub(super) const DESCRIPTORS: &str = "descriptors";

const INTRO: &str = "Agents are the coding CLIs Crowbar runs your chats on - Claude Code, Codex, and any others you add. Crowbar starts one for you when you open a chat, and each keeps its own session and login.";
const REORDER_HINT: &str = "Drag to set which agent a new chat opens first, and turn one off to hide it from every New chat surface. Tools lets that agent use Crowbar itself - reading your workspaces and leaving review comments; turn it off and the agent still runs, with no way in.";

pub(super) fn page() -> Page {
    Page::new("providers", "Agents", "robot")
        .block(Heading::new("Agents", INTRO))
        .block(Section::new("Agents").headerless().row(permission()).row(upload()))
        .block(
            ListBlock::new(AGENTS, "No agents available.")
                .intro(REORDER_HINT)
                .states("Loading agents...", "Crowbar could not load the agent list - it could not reach the daemon.")
                .columns(&["Installed", "Tools", "Enabled"]),
        )
        .block(
            ListBlock::new(DESCRIPTORS, "No descriptors.")
                .titled("Descriptor checks")
                .states("Checking descriptors...", "Crowbar could not check the descriptors - it could not reach the daemon."),
        )
        .block(Section::new("Chat").row(row(
            ("chatIsDefaultPresentation", "Chat By Default"),
            "Open a chat on Chat rather than Terminal. Both surfaces stay in every chat either way - this only picks the one you land on.",
            Control::switch(),
            Value::Bool(true),
        )))
}

fn permission() -> Row {
    row(
        ("defaultPermissionLevel", "Default permission level"),
        "How much of a new chat's tool-call approval is answered automatically.",
        Control::select(
            &[
                ("guarded", "Guarded"),
                ("trusted", "Trusted"),
                ("full-auto", "Full Auto"),
            ],
            ControlWidth::Default,
        ),
        "guarded",
    )
}

fn upload() -> Row {
    let button = crate::button_spec::ButtonSpec::new(
        "Upload agent descriptor",
        Action::backend("agents.uploadDescriptor"),
    )
    .icon("upload");
    Row::new(
        "agentDescriptorUpload",
        "Agent descriptor",
        Control::Button(button),
    )
    .describe(
        "Upload a YAML descriptor with a unique provider ID. Its CLI must be installed separately.",
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::block::Block;

    #[test]
    fn the_page_lists_agents_then_descriptor_checks_then_the_chat_section() {
        let page = page();
        let lists: Vec<&str> = page
            .blocks
            .iter()
            .filter_map(|block| match block {
                Block::List(list) => Some(list.source.as_str()),
                _ => None,
            })
            .collect();
        assert_eq!(lists, [AGENTS, DESCRIPTORS]);
        assert!(matches!(page.blocks.first(), Some(Block::Heading(_))));
        assert!(
            page.rows()
                .any(|row| row.key().as_str() == "defaultPermissionLevel")
        );
    }
}
