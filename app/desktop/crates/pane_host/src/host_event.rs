use protocol::{
    CommandId, EventName, MethodName, PaneFrame, PaneId, PaneState, PaneTitle, PaneTypeId, Params,
    RequestId,
};
use serde_json::Value;

/// Something a pane asked for or the host decided, for the shell to act on. The host never
/// changes the layout itself: it reports, and the shell applies commands to the model.
#[derive(Debug, Clone, PartialEq)]
pub enum HostEvent {
    /// A pane set its tab title.
    TitleChanged {
        /// The pane.
        pane: PaneId,
        /// The new title.
        title: PaneTitle,
    },
    /// A pane replaced its persisted state.
    StateChanged {
        /// The pane.
        pane: PaneId,
        /// The new state, at most 64 KB.
        state: PaneState,
    },
    /// A pane's state must be dropped because its type is destroy-on-hide.
    StateDiscarded {
        /// The pane.
        pane: PaneId,
    },
    /// A pane asked the shell to open another pane.
    OpenRequested {
        /// The asking pane.
        from: PaneId,
        /// The type to open.
        type_id: PaneTypeId,
        /// The ids the new pane is identified by.
        params: Params,
    },
    /// A pane or one of the shell's keybindings triggered a command.
    CommandRequested {
        /// The pane the keypress or call came from.
        pane: PaneId,
        /// The command id.
        command: CommandId,
    },
    /// A pane called its extension backend; answer with [`PaneHost::send`](crate::PaneHost::send).
    RequestMade {
        /// The asking pane.
        pane: PaneId,
        /// Correlation id.
        id: RequestId,
        /// Method name.
        method: MethodName,
        /// Method parameters.
        params: Value,
    },
    /// A pane subscribed to an extension event.
    Subscribed {
        /// The pane.
        pane: PaneId,
        /// Subscription id.
        id: RequestId,
        /// Event name.
        event: EventName,
    },
    /// A pane ended a subscription.
    Unsubscribed {
        /// The pane.
        pane: PaneId,
        /// Subscription id.
        id: RequestId,
    },
    /// A pane sent a frame the shell answers itself: a setting read or subscription, or a
    /// composer or attachment call.
    ShellCall {
        /// The pane.
        pane: PaneId,
        /// The frame as received.
        frame: PaneFrame,
    },
    /// The pointer entered or left a pane's page, which GPUI cannot see over a web view.
    HoverChanged {
        /// The pane.
        pane: PaneId,
        /// Whether the pointer is over it.
        hovered: bool,
    },
}
