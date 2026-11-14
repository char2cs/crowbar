use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::pane_state::PaneState;
use crate::pane_type_id::PaneTypeId;
use crate::params::Params;
use crate::request_id::RequestId;

/// A frame a pane sends to the shell.
///
/// A pane never states who it is: the shell derives the sender from the web view the message
/// arrived on, so no variant carries a pane id.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(
    tag = "kind",
    rename_all = "camelCase",
    rename_all_fields = "camelCase"
)]
pub enum PaneFrame {
    /// `pane.register`: the page announces which pane type it implements.
    #[serde(rename = "pane.register")]
    Register {
        /// The type the page implements.
        type_id: PaneTypeId,
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
        title: String,
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
        method: String,
        /// Method parameters.
        params: Value,
    },
    /// `subscribe`: starts delivery of a manifest-declared event.
    Subscribe {
        /// Subscription id, unique within the pane.
        id: RequestId,
        /// Event name declared in the manifest.
        event: String,
    },
    /// `unsubscribe`: stops a subscription.
    Unsubscribe {
        /// The subscription to stop.
        id: RequestId,
    },
    /// `command.run`: triggers a shell command by id.
    #[serde(rename = "command.run")]
    RunCommand {
        /// Command id.
        command: String,
    },
}
