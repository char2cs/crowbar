//! One file of a review "viewed" mark.

use serde::{Deserialize, Serialize};

/// A changed file and the signature it had when it was marked viewed; the mark holds while the
/// file keeps that signature.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ViewedFile {
    /// Path relative to the repository.
    pub path: String,
    /// `kind|old path|additions|deletions`, as the review computes it.
    pub signature: String,
}
