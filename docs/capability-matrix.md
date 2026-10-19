# Capability and semantics matrix

**Version:** `1.0.0` (`model-check-capability-matrix/1`)  
**Scope:** issue #11 M0a baseline for the `0.4.0` release candidate; `0.3.4` is the last verified release

The [canonical machine-readable matrix](capability-matrix.json) is the only source
of truth for capability identifiers, status, effective semantics, limits, public
surfaces, and regression fixtures. This page is its public entry point; do not copy
rows into another documentation table.

## Status vocabulary

The matrix uses `supported`, `unsupported`, `experimental`, and
`delegated-integrated` to classify product capabilities. These terms are not
verification-result statuses. The CLI and MCP continue to use their established
result and rejection contracts, including `not-executed` and `outside-subset` where
an unsupported input or assumption is encountered.

`delegated-integrated` is reserved for a named M3.0-compliant backend integration.
An external checker mentioned only as an alternative remains `unsupported`.
`experimental` requires a non-default execution path and markers in CLI, MCP, and
report output; version 1 contains no experimental entries.

When an `experimental` entry is introduced, its machine-readable row must include
an `experimental` object with `default_off: true` and `cli_markers`, `mcp_markers`,
and `report_markers` containing the experimental marker asserted by its feature
fixture. A `delegated-integrated` row must instead include a `delegated` object with
the integrated backend identity, its M3.0 contract version, and feature fixtures
that assert both values. The M0 conformance scenario rejects an entry without this
evidence.

## Current boundary

The current supported core is finite, untimed explicit-state verification through the
Promela subset, weighted bounded P/T Petri JSON, or canonical IR. Safety,
reachability, LTL, CTL, progress, POR where eligible, and the documented parallel
paths are listed with their individual restrictions in the canonical matrix.

Timed, probabilistic, symbolic, and unbounded semantics are not product support in
this version. A redirection to another tool does not establish a verification result
or change that classification.

## Issue #11 reconciliation

The accompanying [reconciliation table](../steps/issue-11-reconciliation.md) maps
each explicit issue #11 obligation to a mandatory delivery milestone. It deliberately
keeps the arithmetic-policy selection and any tracker-owner decisions pending rather
than treating an implementer choice as a scope reduction. Therefore this M0a baseline
does not claim that M0, issue #11, or any later milestone is complete.

## Maintenance rule

Every shipped capability change must update the canonical matrix, its linked
regression fixture, the reconciliation table when scope changes, and the public
surface documentation in the same change. The matrix records capabilities already
backed by executable evidence; it never upgrades a documented plan or benchmark into
product support.
