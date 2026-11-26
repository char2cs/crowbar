//! The crate's one journal sink: its events become tracing events under the `browseragent` target.

journal::sink!("browseragent");
