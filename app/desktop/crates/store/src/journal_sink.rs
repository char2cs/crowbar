//! The crate's one journal sink: its events become tracing events under the `store` target.

journal::sink!("store");
