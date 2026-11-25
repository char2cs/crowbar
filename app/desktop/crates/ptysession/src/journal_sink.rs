//! The crate's one journal sink: its events become tracing events under the `ptysession` target.

journal::sink!("ptysession");
