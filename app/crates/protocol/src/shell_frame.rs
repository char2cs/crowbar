use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::event_name::EventName;
use crate::extension_id::ExtensionId;
use crate::extension_status::ExtensionStatus;
use crate::key_binding::KeyBinding;
use crate::lifecycle::Lifecycle;
use crate::pane_id::PaneId;
use crate::pane_state::PaneState;
use crate::params::Params;
use crate::request_id::RequestId;
use crate::rpc_response::RpcResponse;
use crate::setting_key::SettingKey;
use crate::space_id::SpaceId;
use crate::theme_tokens::ThemeTokens;

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
    /// The answer to a `request`, `settings.get` or `composer.draft`.
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
    /// `settings.changed`: a followed setting has a new value.
    #[serde(rename = "settings.changed")]
    SettingChanged {
        /// The `settings.subscribe` this answers.
        id: RequestId,
        /// The setting that changed.
        key: SettingKey,
        /// Its new value.
        value: Value,
    },
    /// `theme`: the design tokens to style with; sent on mount and whenever the theme changes.
    Theme {
        /// The tokens.
        tokens: ThemeTokens,
    },
}
