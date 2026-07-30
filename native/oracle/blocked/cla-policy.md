# Blocked — CLA policy after the AGPL-only relicense

**Item:** 0.12 (relicense to AGPL-3.0-only) · **Status:** needs a user decision
**Raised:** 2026-07-30 · **Does not block anything.** Recorded so it is not lost.

## What happened

D1 killed the commercial licence. `LICENSING.md` previously required a
Contributor License Agreement, and stated its reason explicitly: accepting
outside contributions without one would forfeit the ability to relicense the
tree, since contributors retain copyright in their own work.

That reason is now gone. D1 accepts that the relicensing door closes
permanently the moment GPL code from Zed lands, so there is nothing left for a
CLA to protect *on that ground*.

The worker inferred the obvious conclusion and wrote "no Contributor License
Agreement is required" into `LICENSING.md`. The inference is sound. I removed
it anyway.

## Why it was removed

The reasoning is right and the conclusion may well be what the user wants. But
"no CLA is required" is a **forward-looking commitment to contributors**, and
adding a CLA later would be a visible reversal of a published promise. That is
a policy decision, not a mechanical consequence of the relicense — and per
spec §11.5 a worker does not get to invent one, nor do I.

A CLA can also serve purposes that have nothing to do with relicensing:

- an **explicit patent grant** from contributors (AGPL-3.0 §11 already grants
  patent rights, so this is partly covered — but a CLA can be broader);
- **provenance / right-to-submit** attestation, the job a DCO does;
- **warranty and origin disclaimers** from the contributor to the project.

None of those were the stated reason for the old CLA, and none of them are
addressed by killing the commercial licence.

## Current state of the file

`LICENSING.md` now says the old rationale no longer applies and that the
question is unsettled. It makes **no promise in either direction**. That is
neutral and reversible; both alternatives are not.

## The decision needed

One of:

1. **No CLA, and say so.** Simplest for contributors. Matches the fact that
   relicensing is already foreclosed.
2. **A DCO** (`Signed-off-by`, the Linux/Git model). Very low friction,
   provides provenance, no copyright assignment.
3. **A CLA**, for the patent/provenance/disclaimer reasons above.
4. **Stay silent**, which is where the file sits now.

There is no contribution process in the repo today — there is no
`CONTRIBUTING.md`, and `LICENSING.md` is the only place inbound terms are
stated at all. So this can wait until one is actually needed.
