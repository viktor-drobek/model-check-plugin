---
description: RE2 oracle, fuzzing, and candidate evaluation
paths:
  - cmd/re2loggen/**
  - cmd/difffuzz/**
  - cmd/conformance/**
  - tools/candidates/**
  - scripts/**
  - testdata/re2logs/**
---

# Oracle and candidate tooling

C++ RE2 is the authoritative oracle. Label every oracle import and record RE2
revision, container digest, seed, semantic mode, and corpus hash in generated logs.
Never regenerate tracked logs without an explicit replace action and manifest
update. Differential fuzzing is repository-owned and stores replayable mismatches;
exclusions require a versioned reason and a regression test.

Candidate evaluation accepts only already-present binaries, runs offline in the
specified unprivileged sandbox, and never invokes an LLM. Report build errors,
protocol errors, conformance failures, fuzz mismatches, limits, and timeouts as
distinct outcomes.
