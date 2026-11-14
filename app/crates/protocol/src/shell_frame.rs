use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::extension_id::ExtensionId;
use crate::extension_status::ExtensionStatus;
use crate::key_binding::KeyBinding;
use crate::lifecycle::Lifecycle;
use crate::pane_id::PaneId;
use crate::pane_state::PaneState;
use crate::params::Params;
use crate::request_id::RequestId;
use crate::rpc_error::RpcError;
use crate::space_id::SpaceId;

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
    /// The answer to a `request`.
    Response {
        /// The request being answered.
        id: RequestId,
        /// Result on success.
        #[serde(skip_serializing_if = "Option::is_none")]
        result: Option<Value>,
        /// Failure otherwise.
        #[serde(skip_serializing_if = "Option::is_none")]
        error: Option<RpcError>,
    },
    /// `extension.event`: an event an extension backend emitted.
    #[serde(rename = "extension.event")]
    ExtensionEvent {
        /// The emitting extension.
        extension_id: ExtensionId,
        /// Optional Space scope.
        #[serde(skip_serializing_if = "Option::is_none")]
        space_id: Option<SpaceId>,
        /// Event name.
        name: String,
        /// Event payload.
        payload: Value,
    },
    /// `extension.status`: the state of an extension changed.
    #[serde(rename = "extension.status")]
    ExtensionStatus(ExtensionStatus),
    /// The active keybindings; the pane forwards matching keydowns as `command.run`.
    Keybindings {
        /// Active bindings.
        bindings: Vec<KeyBinding>,
    },
}
