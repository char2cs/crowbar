use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::attachment_id::AttachmentId;
use crate::attachment_type::AttachmentType;
use crate::command_id::CommandId;
use crate::event_name::EventName;
use crate::external_url::ExternalUrl;
use crate::fs_path::FsPath;
use crate::method_name::MethodName;
use crate::pane_state::PaneState;
use crate::pane_title::PaneTitle;
use crate::pane_type_id::PaneTypeId;
use crate::params::Params;
use crate::request_id::RequestId;
use crate::sdk_version::SdkVersion;
use crate::setting_key::SettingKey;

/// A frame a pane sends to the shell.
///
/// A pane never states who it is: the shell derives the sender from the web view the message
/// arrived on, so no variant carries a pane id. Calls that expect an answer carry a
/// [`RequestId`] and are answered by a [`ShellFrame::Response`](crate::ShellFrame::Response).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(
    tag = "kind",
    rename_all = "camelCase",
    rename_all_fields = "camelCase"
)]
pub enum PaneFrame {
    /// `pane.register`: the page announces which pane type it implements and which SDK
    /// version it was built for; the shell settles on [`SdkVersion::negotiate`]'s answer.
    #[serde(rename = "pane.register")]
    Register {
        /// The type the page implements.
        type_id: PaneTypeId,
        /// The SDK version the page speaks, named apart from the envelope's own `sdk` key.
        #[serde(rename = "sdkVersion")]
        sdk: SdkVersion,
    },
    /// `pane.open`: asks the shell to open another pane.
    #[serde(rename = "pane.open")]
    Open {
        /// The type to open.
        type_id: PaneTypeId,
        /// The ids the new pane is identified by.
        params: Params,
    },
    /// `pane.setTitle`: sets the pane's tab title.
    #[serde(rename = "pane.setTitle")]
    SetTitle {
        /// The new title.
        title: PaneTitle,
    },
    /// `pane.setState`: replaces the pane's persisted state.
    #[serde(rename = "pane.setState")]
    SetState {
        /// The new state, at most 64 KB.
        state: PaneState,
    },
    /// `request`: a call routed to the pane's own extension backend.
    Request {
        /// Correlation id echoed by the `response` frame.
        id: RequestId,
        /// Method name declared in the manifest.
        method: MethodName,
        /// Method parameters.
        params: Value,
    },
    /// `subscribe`: starts delivery of a manifest-declared event.
    Subscribe {
        /// Subscription id, unique within the pane.
        id: RequestId,
        /// Event name declared in the manifest.
        event: EventName,
    },
    /// `unsubscribe`: stops an event or settings subscription.
    Unsubscribe {
        /// The subscription to stop.
        id: RequestId,
    },
    /// `command.run`: triggers a shell command by id.
    #[serde(rename = "command.run")]
    RunCommand {
        /// Command id.
        command: CommandId,
    },
    /// `settings.get`: reads one setting; answered by a `response`.
    #[serde(rename = "settings.get")]
    SettingsGet {
        /// Correlation id echoed by the `response` frame.
        id: RequestId,
        /// The setting to read.
        key: SettingKey,
    },
    /// `settings.subscribe`: delivers `settings.changed` frames for one setting until the
    /// subscription is stopped with `unsubscribe`.
    #[serde(rename = "settings.subscribe")]
    SettingsSubscribe {
        /// Subscription id, unique within the pane.
        id: RequestId,
        /// The setting to follow.
        key: SettingKey,
    },
    /// `composer.attach`: adds an attachment to the composer's draft.
    #[serde(rename = "composer.attach")]
    ComposerAttach {
        /// The kind of attachment.
        attachment_type: AttachmentType,
        /// The attachment's data, bounded by the frame size cap.
        payload: Value,
    },
    /// `composer.draft`: reads a read-only summary of the draft; answered by a `response`.
    #[serde(rename = "composer.draft")]
    ComposerDraft {
        /// Correlation id echoed by the `response` frame.
        id: RequestId,
    },
    /// `attachment.open`: opens an attachment in its editor pane.
    #[serde(rename = "attachment.open")]
    AttachmentOpen {
        /// The attachment to open.
        attachment_id: AttachmentId,
    },
    /// `shell.openExternal`: opens a web, mail or phone link in the system's default
    /// application. Nothing is answered; a refused link is only logged.
    #[serde(rename = "shell.openExternal")]
    OpenExternal {
        /// The link.
        url: ExternalUrl,
    },
    /// `shell.reveal`: shows a file or directory in the system file manager. Nothing is
    /// answered; a path that does not exist is only logged.
    #[serde(rename = "shell.reveal")]
    Reveal {
        /// The absolute path.
        path: FsPath,
    },
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_shell_service_frames_round_trip() {
        let cases = [
            r#"{"kind":"shell.openExternal","url":"https://example.com/"}"#,
            r#"{"kind":"shell.reveal","path":"/tmp/a b"}"#,
        ];
        for text in cases {
            let frame: PaneFrame = serde_json::from_str(text).unwrap();
            assert_eq!(serde_json::to_string(&frame).unwrap(), text);
        }
    }

    #[test]
    fn a_service_frame_with_a_bad_argument_is_refused() {
        let cases = [
            r#"{"kind":"shell.openExternal","url":"file:///etc/passwd"}"#,
            r#"{"kind":"shell.reveal","path":"relative"}"#,
        ];
        for text in cases {
            assert!(serde_json::from_str::<PaneFrame>(text).is_err(), "{text}");
        }
    }
}
