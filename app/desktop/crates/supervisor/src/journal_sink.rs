//! The crate's one journal sink: its events become tracing events under the `supervisor` target.

journal::sink!("supervisor");
