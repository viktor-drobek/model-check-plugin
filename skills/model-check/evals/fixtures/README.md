# Fixtures: references, not copies

The models used by `evals/evals.json` belong to the corpus `Promela - examples/`
(Holzmann, *The SPIN Model Checker*, 2003). They are **not copied** into the skill
(plan 14 §2.1, licence note): a fixture is a repository path plus the SHA-256 hash
of the file's content. An eval runner resolves the path relative to the repository
root and refuses to run if the hash differs — the expectations were written against
this exact content.

Hashes computed with `sha256sum` on 2026-09-24.

| Eval | Fixture path (repository root) | SHA-256 | Bytes |
|---|---|---|---|
| E1 | `Promela - examples/CH2/mutex_flaw.pml` | `b9cb0230f9a306494af4a12592df3411988254b35526b5fbcdbf17ab328fdac4` | 391 |
| E2 | `Promela - examples/CH3/alternatingbit.pml` | `09006d65d6619931809544f074da440a9f3758e7715d7cc7493657164267698d` | 321 |
| E3 | `Promela - examples/App_C/petrinet1` (reference encoding; the eval prompt gives the net in words and the runner compares the built JSON against it) | `b20970178bb7adfec73a131d56c7f9e6fc00e7ec07c995f9a1c7285f60d81d87` | 526 |
| E4, E6 | `Promela - examples/CH14/version1` | `acdfcacad083c29ff47cba18de2fb84ec784e9e064e0b64c435497e07121ec3b` | 752 |
| E5 | `Promela - examples/CH17/simple1.pr` | `d7a34d649c9cc83ecb852dd2ca584c1ce3b977fceb5a2dbb5d09db542b2d4b70` | 184 |

Golden expectations that will be attached when the evals half runs (plan §8.1,
§8.2): `petrinet1` → deadlock after `t1, t4` with `p2 = p5 = 1`; `mutex_flaw` →
invariant violated through `L1`–`L4`; `simple1.pr` → parser rejection (`c_code`),
status `not-executed`. `App_C/petrinet2`
(`19aff7aa4b92b7db14999b9339aeb95fe17c251f36e71a2c3be1b5ea2df2a1b5`) gets a golden
only after the first agreed engine/SPIN run.

How to add a fixture: add a row with path, hash and size; never add the file itself.
Models from the second corpus (lectures 07, Karpov 03, Velder 09 — extracted from
`books-md/`) follow the same rule with their `books-md/` source path and the line
range of the listing.
