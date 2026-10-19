# Offline standards imports

Stage 3a of issue #18 adds in-memory adapters for two standards artefacts:

- a caller-provided **RFC text** with a declared RFC Editor or IETF HTTPS source;
- a caller-provided **IEEE clause manifest**, not an IEEE PDF.

Both adapters produce `mcd-trace-ledger/1` documents. They do not fetch from the
network, open the logical input path, run external tools, download a PDF, or make an
implementation-correctness claim.

## RFC text

```go
ledger, err := standards.ImportRFC(standards.RFCInput{
    Artifact: standards.Input{
        Path:    "standards/rfc-8174.txt", // stable logical anchor only
        Content: rfcText,                    // supplied bytes
    },
    SourceURL: "https://www.rfc-editor.org/rfc/rfc8174.txt",
})
```

`ImportRFC` accepts only the canonical HTTPS RFC-text endpoints
`https://www.rfc-editor.org/rfc/rfc<number>.txt` and
`https://www.ietf.org/rfc/rfc<number>.txt`. It rejects any other host spelling,
path, percent-encoding, userinfo, query, fragment, port, leading or trailing
whitespace, or alternate scheme casing. The original input string must already be
one of those canonical serializations; the adapter does not normalize it. The URL number must
match the declared `Request for Comments: <number>`; for example, RFC 8174 must
declare `.../rfc8174.txt`, not a draft, errata page, mailing-list message, or
another RFC's URL. It reads headers only from the leading, column-zero header block
and requires both `Request for Comments: <number>` and a `Category`. Required headers
must have exactly one ASCII space after their colon, use only ASCII spaces or tabs as layout whitespace, and contain no Unicode whitespace.
`Category` must be exactly one of `Standards Track`, `Best Current Practice`,
`Informational`, `Experimental`, `Historic`, or `FYI` after removal of a genuine
right-hand column. An explicit date is a genuine right-hand column; a numeric or
category-valued column is ambiguous and rejected rather than truncated.

Before parsing, CRLF and CR line endings are normalized to LF. The result preserves
the RFC number, publication category (rendered as `RFC <number>, category <value>`),
optional `Updates` header, source URL plus logical-path and line anchors
(`#logical-path=<path>&line=<n>`), the SHA-256 of the supplied text, and the adapter
producer/version.
A comma-terminated `Updates` list may continue on indented header lines; malformed
or incomplete lists are rejected. The complete wrapped list is capped at 100 RFC
numbers. A comma-terminated list may end before a right-hand column only when that
column is an explicit date such as `1 April 2026`; a column whose first token is a
bare RFC number (including a four-digit number), an RFC-number list, or any comma in
the right-hand column is rejected rather than silently discarded. A date column does
not terminate the list when an indented continuation line follows it. A whitespace-only
indented line is a header separator, not a continuation. Header metadata and an
optional caller-supplied source revision are checked for control and Unicode format
characters and surrounding whitespace before any normalization.

The adapter emits `Requirement` nodes with `normative_candidate` evidence only
after an in-document paragraph states that key words **are to be interpreted as
described in** BCP 14, RFC 2119, or RFC 8174. A capitalised `MUST` before or
without that specific declaration — including an unrelated reference to an RFC or a
title that mentions key words — creates no candidate. The leading RFC header block
is metadata and never supplies that declaration. Template text directly following an
author or editor instruction at the paragraph start or after a sentence boundary
(including a `who` or `that` relative clause) using the lowercase boilerplate verbs
`may`, `can`, or `should` to incorporate, include, or use a phrase, text, or
boilerplate is not an in-document declaration. Uppercase BCP 14 verbs remain
operational content. This template exclusion applies across
separator lines, recognised page furniture, and same-paragraph wording. Page furniture
is itself excluded from candidates only when a raw single line has the canonical
column-aligned `[Page N]` footer shape or a canonical RFC/Internet-Draft multi-column
running-header shape with a month-and-year final column; ordinary body prose remains
eligible for extraction. Explicit
replacement labels include `OLD:`, `OLD TEXT:`, `NEW:`, `NEW TEXT:`, `REPLACEMENT:`,
and `QUOTED TEXT:` at a sentence boundary. An explicitly labelled replacement block
remains quoted through the end of the input and is excluded from candidates. A narrow
RFC/BCP quotation lead-in such as `RFC 2119 states:` or `[RFC2119] states:`
excludes its following paragraph, including when the lead-in shares a paragraph with
a declaration. The RFC number may be any positive RFC number, but the citation must
be immediately followed by a reporting verb and colon; template context is consumed
by its next non-furniture paragraph. Every matching declaration
paragraph is excluded from candidates, so a mandate in that same paragraph is
deliberately not emitted; place a requirement in a later paragraph for this adapter
to record it. Candidates are one post-declaration physical line at a time. Tabs in a
candidate line become spaces in its label, while candidate lines containing control
or Unicode format characters are rejected. Candidates are not reviewer-approved
requirements and do not establish any source or model claim.

The current increment accepts RFC text only. It does not parse RFC XML or retrieve
or update an RFC on behalf of the caller.

## IEEE clause manifest

An IEEE input is a small JSON manifest that may contain only the standard identity,
its edition, the SHA-256 of the licensed document, and concise clause summaries:

```json
{
  "standard_id": "IEEE 802.1Q",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": [
    {
      "anchor": "8.6.8",
      "summary": "The bridge relay function forwards a frame only according to the declared forwarding rule."
    }
  ]
}
```

Both adapters accept a clean, unpadded relative logical input path only and reject an input
larger than 1 MiB. `ImportRFC` also rejects more than 10,000 candidate requirement
lines. `ImportIEEEManifest` rejects more than 1,000 clauses, duplicate keys, unknown fields (including
case-variant spellings), trailing JSON, invalid or non-lowercase hashes, blank or
duplicated clause anchors, overlong identifiers, editions, or anchors, and summaries
longer than 500 Unicode code points. Clause anchors accept only letters, digits,
periods, underscores, parentheses, and hyphens. It records a `StandardClause`, its derived
`Requirement` summary, and a `derived_from` relation. Every record is pinned to the
licensed document hash and marked `licensed_clause_reference`.
The provenance source revision also always includes
`manifest-sha256:<digest>` for the supplied manifest bytes. A caller-supplied
revision follows only as `;rev=<revision>`, so the digest has a fixed,
unambiguous position. An IEEE caller revision must not contain
`manifest-sha256:` because that marker is reserved for the adapter-generated
prefix.

The adapter deliberately does not accept an IEEE PDF, document path, or document
URL. The manifest is a concise user-provided reference, not a redistribution of
licensed material or a legal interpretation. Every metadata field must be canonical
and have no leading or trailing whitespace.

## Remaining Stage 3 scope

TShark JSON dissections, Kaitai/Spicy grammar references, RFC XML, and all capture
handling are still deferred. In particular, this increment does not analyse live
traffic, infer a protocol state machine, or add a grammar-artifact node type to the
versioned trace schema.
