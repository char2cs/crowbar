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
use crate::control::payload::Params;
use crate::control::payload::ViewedFile;
use crate::control::sdk_version::SdkVersion;
use crate::identity::CommandId;
use crate::identity::PaneTypeId;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(
    tag = "kind",
    rename_all = "camelCase",
    rename_all_fields = "camelCase"
)]
pub enum PaneFrame {
    #[serde(rename = "pane.register")]
    Register {
        type_id: PaneTypeId,
        #[serde(rename = "sdkVersion")]
        sdk: SdkVersion,
    },
    #[serde(rename = "pane.open")]
    Open(PaneOpen),
    #[serde(rename = "pane.setTitle")]
    SetTitle {
        title: PaneTitle,
    },
    #[serde(rename = "pane.setParams")]
    SetParams {
        params: Params,
    },
    #[serde(rename = "pane.setState")]
    SetState {
        state: PaneState,
    },
    #[serde(rename = "pane.setDirty")]
    SetDirty {
        dirty: bool,
    },
    Request {
        id: RequestId,
        method: MethodName,
        params: Value,
    },
    Subscribe {
        id: RequestId,
        event: EventName,
    },
    Unsubscribe {
        id: RequestId,
    },
    #[serde(rename = "command.run")]
    RunCommand {
        command: CommandId,
    },
    #[serde(rename = "settings.set")]
    SettingsSet {
        version: u32,
        key: SettingKey,
        value: Value,
    },
    #[serde(rename = "shell.openExternal")]
    OpenExternal {
        url: ExternalUrl,
    },
    #[serde(rename = "review.setViewed")]
    ReviewSetViewed {
        files: Vec<ViewedFile>,
        viewed: bool,
    },
    #[serde(rename = "shell.reveal")]
    Reveal {
        path: FsPath,
    },
    #[serde(rename = "pane.openRefused")]
    OpenRefused {
        #[serde(default, skip_serializing_if = "Option::is_none")]
        title: Option<DisplayText>,
        message: DisplayText,
    },
    #[serde(rename = "shell.toast")]
    Toast(PaneToast),
    #[serde(rename = "document.edit")]
    DocumentEdit {
        text: String,
    },
    #[serde(rename = "document.showSource")]
    DocumentShowSource,
    #[serde(rename = "document.save")]
    DocumentSave,
    #[serde(rename = "shell.toastClose")]
    ToastClose {
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
            r#"{"kind":"pane.setParams","params":{"threadId":"c1"}}"#,
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
            r#"{"kind":"pane.setParams","params":{"threadId":""}}"#,
        ];
        for text in cases {
            assert!(serde_json::from_str::<PaneFrame>(text).is_err(), "{text}");
        }
    }
}
