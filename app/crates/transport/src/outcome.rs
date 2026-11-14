use crate::ending::Ending;

#[derive(Debug)]
pub(crate) enum Outcome {
    Upload(Ending),
    Download(Ending),
    Revoked,
}
