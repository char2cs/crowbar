use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::control::call::EventName;
use crate::control::call::MethodName;
use crate::control::call::RequestId;
use crate::control::config::SettingKey;
use crate::control::payload::DisplayText;
use crate::control::payload::ExternalUrl;
use crate::control::payload::FsPath;
use crate::control::payload::PaneOpen;
use crate::control::payload::PaneState;
use crate::control::payload::PaneTitle;
use crate::control::payload::PaneToast;
use crate::control::payload::ViewedFile;
use crate::control::sdk_version::SdkVersion;
use crate::identity::CommandId;
use crate::identity::PaneTypeId;

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
    Open(PaneOpen),
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
    /// `pane.setDirty`: tells the shell whether the pane holds work that closing would lose.
    /// The shell asks before it closes a dirty pane and sends [`ShellFrame::Save`](crate::ShellFrame::Save)
    /// when the user chooses to save.
    #[serde(rename = "pane.setDirty")]
    SetDirty {
        /// Whether the pane has unsaved work.
        dirty: bool,
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
    /// `settings.set`: asks the shell to store one of the settings a `settings` frame carries.
    /// The shell stays the owner: the pane sees the new value when the shell pushes the set
    /// again, and a key outside the frame's vocabulary or a `version` the shell does not speak
    /// is ignored.
    #[serde(rename = "settings.set")]
    SettingsSet {
        /// The vocabulary version, [`SETTINGS_FRAME_VERSION`](crate::SETTINGS_FRAME_VERSION).
        version: u32,
        /// The setting to store.
        key: SettingKey,
        /// Its new value.
        value: Value,
    },
    /// `shell.openExternal`: opens a web, mail or phone link in the system's default
    /// application. Nothing is answered; a refused link is only logged.
    #[serde(rename = "shell.openExternal")]
    OpenExternal {
        /// The link.
        url: ExternalUrl,
    },
    /// `review.setViewed`: asks the shell, which owns the review's viewed marks, to mark or unmark
    /// files of the pane's own workspace. Nothing is answered; the pane sees the result in the
    /// next [`ShellFrame::ReviewViewed`](crate::ShellFrame::ReviewViewed).
    #[serde(rename = "review.setViewed")]
    ReviewSetViewed {
        /// The files and their current signatures.
        files: Vec<ViewedFile>,
        /// Mark them viewed, or clear their marks.
        viewed: bool,
    },
    /// `shell.reveal`: shows a file or directory in the system file manager. Nothing is
    /// answered; a path that does not exist is only logged.
    #[serde(rename = "shell.reveal")]
    Reveal {
        /// The absolute path.
        path: FsPath,
    },
    /// `pane.openRefused`: a pane opened by the user could not read what it was opened on. The
    /// shell closes the pane and says why, as a toast titled `title` when one is given.
    #[serde(rename = "pane.openRefused")]
    OpenRefused {
        /// What failed, such as "File not found".
        #[serde(default, skip_serializing_if = "Option::is_none")]
        title: Option<DisplayText>,
        /// The detail, such as the file's path.
        message: DisplayText,
    },
    /// `shell.toast`: shows a toast in the shell's toast host. A toast already shown under the
    /// same key by this pane is rewritten in place. Nothing is answered.
    #[serde(rename = "shell.toast")]
    Toast(PaneToast),
    /// `document.edit`: the page's view of its file pane's document now holds `text`. The shell
    /// owns the text and takes it as one edit; dirty state and saving stay with the shell.
    #[serde(rename = "document.edit")]
    DocumentEdit {
        /// The whole text.
        text: String,
    },
    /// `document.showSource`: asks the shell to show its file pane's document in the source
    /// editor instead of the page.
    #[serde(rename = "document.showSource")]
    DocumentShowSource,
    /// `document.save`: the user asked to save the file pane's document (mod+s in the page). The
    /// page sends its pending edit first; the shell saves what it then holds.
    #[serde(rename = "document.save")]
    DocumentSave,
    /// `shell.toastClose`: takes down the toast this pane showed under `key`.
    #[serde(rename = "shell.toastClose")]
    ToastClose {
        /// The pane's id for the toast.
        key: DisplayText,
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
            r#"{"kind":"review.setViewed","files":[{"path":"src/a.ts","signature":"M||1|0"}],"viewed":true}"#,
            r#"{"kind":"shell.toast","key":"t1","tone":"error","title":"Save the file","description":"disk full","pinned":false}"#,
            r#"{"kind":"shell.toast","key":"t2","tone":"loading","title":"Taking the prompt back…","pinned":true}"#,
            r#"{"kind":"shell.toastClose","key":"t2"}"#,
            r#"{"kind":"pane.openRefused","title":"File not found","message":"src/a.ts"}"#,
            r#"{"kind":"pane.openRefused","message":"This project is no longer in your list"}"#,
            r#"{"kind":"document.edit","text":"Title\n"}"#,
            r#"{"kind":"document.showSource"}"#,
            r#"{"kind":"document.save"}"#,
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
            r#"{"kind":"shell.toast","key":"t1","tone":"shout","title":"x"}"#,
        ];
        for text in cases {
            assert!(serde_json::from_str::<PaneFrame>(text).is_err(), "{text}");
        }
    }
}
