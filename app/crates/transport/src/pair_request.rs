/// What the user typed and confirmed in the pairing dialog.
#[derive(Debug, Clone, Copy)]
pub struct PairRequest<'a> {
    /// The id to register the host under.
    pub host_id: &'a str,
    /// The host's `host:port`.
    pub endpoint: &'a str,
    /// The fingerprint the user compared against `crowbar pair`.
    pub fingerprint: &'a str,
    /// The one-time pairing code.
    pub code: &'a str,
    /// A name the host shows for this client.
    pub name: &'a str,
}
