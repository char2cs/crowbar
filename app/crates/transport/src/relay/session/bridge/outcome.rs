use crate::relay::session::bridge::ending::Ending;

#[derive(Debug)]
pub(crate) enum Outcome {
    Upload(Ending),
    Download(Ending),
    Revoked,
}
