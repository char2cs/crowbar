use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::control::call::EventName;
use crate::control::call::RpcResponse;
use crate::control::config::KeyBinding;
use crate::control::config::SettingValues;
use crate::control::config::ThemeTokens;
use crate::control::lifecycle::Lifecycle;
use crate::control::payload::DisplayText;
use crate::control::payload::ExtensionStatus;
use crate::control::payload::PaneState;
use crate::control::payload::Params;
use crate::identity::ExtensionId;
use crate::identity::PaneId;
use crate::identity::SpaceId;

/// A frame the shell sends to a pane.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(
    tag = "kind",
    rename_all = "camelCase",
    rename_all_fields = "camelCase"
)]
pub enum ShellFrame {
    /// A lifecycle event carrying everything the pane needs to render.
    Lifecycle {
        /// The pane the event is for.
        pane_id: PaneId,
        /// What happened.
        event: Lifecycle,
        /// The ids the pane is identified by.
        params: Params,
        /// The pane's persisted state.
        state: PaneState,
    },
    /// `pane.save`: the user chose to save before the shell closes the pane. The pane saves and
    /// reports [`PaneFrame::SetDirty`](crate::PaneFrame::SetDirty) `false`; a failed save leaves
    /// it dirty and the pane stays open.
    #[serde(rename = "pane.save")]
    Save,
    /// The answer to a `request`.
    Response(RpcResponse),
    /// `extension.event`: an event an extension backend emitted.
    #[serde(rename = "extension.event")]
    ExtensionEvent {
        /// The emitting extension.
        extension_id: ExtensionId,
        /// Optional Space scope.
        #[serde(skip_serializing_if = "Option::is_none")]
        space_id: Option<SpaceId>,
        /// Event name.
        name: EventName,
        /// Event payload.
        payload: Value,
    },
    /// `extension.status`: the state of an extension changed.
    #[serde(rename = "extension.status")]
    ExtensionStatus(ExtensionStatus),
    /// The active keybindings; the pane forwards matching keydowns as `command.run`. The shell
    /// sends the whole list again whenever it changes.
    Keybindings {
        /// Active bindings.
        bindings: Vec<KeyBinding>,
    },
    /// `protocol.error`: the shell refused one frame and kept the pane. The pane learns why
    /// instead of waiting for an answer that will not come.
    #[serde(rename = "protocol.error")]
    ProtocolError {
        /// What was wrong with the frame.
        message: DisplayText,
    },
    /// `theme`: the design tokens to style with; sent on mount and whenever the theme changes.
    Theme {
        /// The tokens.
        tokens: ThemeTokens,
    },
    /// `group`: how the pane's group is arranged; sent to a chat on mount and whenever it
    /// changes. A chat that fills its group offers to open the editor beside it.
    Group {
        /// Whether the chat fills its group: no editor tab and the editor view closed.
        chat_fills: bool,
    },
    /// `settings`: the settings every pane follows, sent on mount and whenever one changes. It
    /// is the whole set each time: a key that is missing has its default.
    Settings {
        /// The vocabulary version, [`SETTINGS_FRAME_VERSION`](crate::SETTINGS_FRAME_VERSION).
        version: u32,
        /// The values.
        values: SettingValues,
    },
    /// `review.viewed`: a workspace's viewed marks, path to signature; sent to its review panes
    /// on mount and whenever a mark changes. It is the whole map each time.
    #[serde(rename = "review.viewed")]
    ReviewViewed {
        /// The workspace.
        workspace: String,
        /// Path to the signature the file had when it was marked.
        viewed: BTreeMap<String, String>,
    },
    /// `review.reveal`: show this changed file on the review's Files view.
    #[serde(rename = "review.reveal")]
    ReviewReveal {
        /// Path relative to the repository.
        path: String,
    },
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_review_frames_have_the_shape_the_pane_sdk_reads() {
        let cases = [
            r#"{"kind":"review.viewed","workspace":"w1","viewed":{"src/a.ts":"M||1|0"}}"#,
            r#"{"kind":"review.reveal","path":"src/a.ts"}"#,
        ];
        for text in cases {
            let frame: ShellFrame = serde_json::from_str(text).unwrap();
            assert_eq!(serde_json::to_string(&frame).unwrap(), text);
        }
    }
}
