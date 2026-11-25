//! The crate's one journal sink: its events become tracing events under the `fake_daemon` target.

journal::sink!("fake_daemon");
