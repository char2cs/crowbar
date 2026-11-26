//! The crate's one journal sink: its events become tracing events under the `transport` target.

journal::sink!("transport");
