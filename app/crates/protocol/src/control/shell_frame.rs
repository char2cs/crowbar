use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::control::call::EventName;
use crate::control::call::RpcResponse;
use crate::control::config::KeyBinding;
use crate::control::config::SettingValues;
use crate::control::config::ThemeTokens;
use crate::control::lifecycle::Lifecycle;
use crate::control::payload::Breakpoint;
use crate::control::payload::DisplayText;
use crate::control::payload::ExtensionStatus;
use crate::control::payload::PairedHost;
use crate::control::payload::PaneState;
use crate::control::payload::Params;
use crate::identity::ExtensionId;
use crate::identity::PaneId;
use crate::identity::SpaceId;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(
    tag = "kind",
    rename_all = "camelCase",
    rename_all_fields = "camelCase"
)]
pub enum ShellFrame {
    Lifecycle {
        pane_id: PaneId,
        event: Lifecycle,
        params: Params,
        state: PaneState,
    },
    #[serde(rename = "pane.save")]
    Save,
    Response(RpcResponse),
    #[serde(rename = "extension.event")]
    ExtensionEvent {
        extension_id: ExtensionId,
        #[serde(skip_serializing_if = "Option::is_none")]
        space_id: Option<SpaceId>,
        name: EventName,
        payload: Value,
    },
    #[serde(rename = "extension.status")]
    ExtensionStatus(ExtensionStatus),
    Keybindings {
        bindings: Vec<KeyBinding>,
    },
    #[serde(rename = "protocol.error")]
    ProtocolError {
        message: DisplayText,
    },
    Theme {
        tokens: ThemeTokens,
    },
    Breakpoints {
        cleared: Vec<Breakpoint>,
    },
    #[serde(rename = "chat.zoom")]
    ChatZoom {
        percent: u16,
    },
    Group {
        chat_fills: bool,
    },
    Settings {
        version: u32,
        values: SettingValues,
    },
    Hosts {
        hosts: Vec<PairedHost>,
    },
    #[serde(rename = "review.viewed")]
    ReviewViewed {
        workspace: String,
        viewed: BTreeMap<String, String>,
    },
    #[serde(rename = "review.reveal")]
    ReviewReveal {
        path: String,
    },
    #[serde(rename = "document.text")]
    DocumentText {
        text: String,
    },
    #[serde(rename = "files.dropped")]
    FilesDropped {
        paths: Vec<String>,
        x: f64,
        y: f64,
    },
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_review_and_breakpoint_frames_have_the_shape_the_pane_sdk_reads() {
        let cases = [
            r#"{"kind":"review.viewed","workspace":"w1","viewed":{"src/a.ts":"M||1|0"}}"#,
            r#"{"kind":"review.reveal","path":"src/a.ts"}"#,
            r#"{"kind":"breakpoints","cleared":["sm","md"]}"#,
            r#"{"kind":"chat.zoom","percent":120}"#,
            r#"{"kind":"hosts","hosts":[{"id":"h1","name":"rigremote","endpoint":"127.0.0.1:4747"}]}"#,
            r#"{"kind":"document.text","text":"Title\n"}"#,
            r#"{"kind":"files.dropped","paths":["/tmp/a b.rs"],"x":12.5,"y":40.0}"#,
        ];
        for text in cases {
            let frame: ShellFrame = serde_json::from_str(text).unwrap();
            assert_eq!(serde_json::to_string(&frame).unwrap(), text);
        }
    }
}
