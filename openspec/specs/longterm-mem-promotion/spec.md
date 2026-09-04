# Longterm-Mem Promotion Specification

## Purpose

Defines the promotion writer: which Engram observations are eligible, how an
eligible observation is emitted as a contract-conformant vault page, how it
is addressed and registered, how it stays mutable and current without
clobbering a local edit, how `sync` and an explicit `promote` call trigger
it, and how retraction (soft-delete or supersession) propagates without ever
rewriting a page's body.

## Requirements

### Requirement: Promotion Eligibility Predicate

ID: R-007
Traces to: longterm-mem R-007

WHILE promotion runs for project P, the longterm-mem promotion writer SHALL
treat an Engram observation as eligible only if it is pinned, OR is of type
`decision`, `architecture`, or `pattern`, OR has a revision count of at
least 3, OR is explicitly targeted by a promote call.

#### Scenario: Pinned observation is eligible

- GIVEN an observation that is pinned
- WHEN eligibility is evaluated
- THEN it is eligible

#### Scenario: High-revision, untyped, unpinned observation is eligible

- GIVEN an observation of type `discovery` with a revision count of 4 and
  not pinned
- WHEN eligibility is evaluated
- THEN it is eligible

#### Scenario: Low-revision, untyped, unpinned observation is not eligible

- GIVEN an observation of type `discovery` with a revision count of 1, not
  pinned, not explicitly targeted
- WHEN eligibility is evaluated
- THEN it is not eligible

#### Scenario: Explicit promote call overrides the automatic criteria

- GIVEN an observation named by an explicit promote call
- WHEN eligibility is evaluated for that call
- THEN it is eligible regardless of type, pin state, or revision count

### Requirement: Contract-Conformant Page Emission

ID: R-027
Traces to: longterm-mem R-027

WHEN an eligible observation is promoted, the promotion writer SHALL emit a
vault page with universal frontmatter plus flat extras identifying the
Engram source (its id, its type mapped onto the vault's own type contract,
and its project), with the page's related-links field populated from
Engram's relation graph and any already-promoted counterpart such that every
wikilink resolves, using an address-first filename that survives a later
retitle.

#### Scenario: Type is mapped onto the vault's contract enum

- GIVEN an eligible `decision`-typed observation
- WHEN it is promoted
- THEN the emitted page's frontmatter type value is a value inside the
  vault's own contract enum, with the raw Engram type, Engram id, and
  project present as flat extras

#### Scenario: Related links resolve

- GIVEN the observation has one relation edge to another already-promoted
  observation
- WHEN it is promoted
- THEN the related-links field includes a link to that other page, and that
  link resolves to an existing file

#### Scenario: Filename survives a retitle

- GIVEN the observation's filename is derived from its allocated address
  rather than its title
- WHEN the observation is later retitled in Engram
- THEN the promoted page's filename is unchanged

#### Scenario: Freshly promoted page passes the vault's own lint

- GIVEN a freshly promoted page
- WHEN the vault's own lint check runs over it
- THEN it passes

### Requirement: Address Allocation and Manifest Registration

ID: R-028
Traces to: longterm-mem R-028

WHEN a page is emitted for an eligible observation, the promotion writer
SHALL allocate a vault address for it and record that address in the
vault's own address and source manifest.

#### Scenario: First promotion allocates a new address

- GIVEN a newly promoted observation
- WHEN its page is emitted
- THEN a new address is allocated and the vault's own address and source
  manifest references it

#### Scenario: Re-promotion reuses the existing address

- GIVEN a re-promotion of an already-addressed observation
- WHEN it runs
- THEN no new address is allocated and the existing one is reused

### Requirement: Index and Log Registration

ID: R-029
Traces to: longterm-mem R-029

WHEN a page is emitted for an eligible observation, the promotion writer
SHALL register it in both the vault's master catalog and its append-only
promotion log.

#### Scenario: New page is discoverable and logged

- GIVEN a newly promoted page
- WHEN promotion completes
- THEN the vault's master catalog lists it and its append-only log records
  the promotion event

### Requirement: Update-in-Place on Revision

ID: R-008
Traces to: longterm-mem R-008

WHEN a previously promoted Engram observation is revised or re-promoted, the
longterm-mem promotion writer SHALL update the existing vault page in place,
subject to the local-edit precedence rule, rather than creating a duplicate
page.

#### Scenario: Unmodified page updates in place on revision

- GIVEN observation X was previously promoted to vault page V and has not
  been locally modified since
- WHEN X's revision count increases and promotion runs again
- THEN V's content and its updated timestamp are refreshed, and no second
  page for X is created

#### Scenario: Retitle keeps the same file

- GIVEN observation X's Engram title changed since its last promotion but
  its Engram id is unchanged
- WHEN promotion runs again
- THEN the same on-disk page is updated with the new title, no new file is
  created, and no old file is orphaned

### Requirement: Local-Edit Precedence

ID: R-030
Traces to: longterm-mem R-030

IF a promoted vault page's on-disk content diverges from what longterm-mem
itself last wrote for that page, THEN the promotion writer SHALL skip that
page's content update and emit a diagnostic instead, tracked through a
sidecar precedence store keyed by the page's address.

#### Scenario: Locally edited page is skipped with a diagnostic

- GIVEN a promoted page a human or agent edited directly in the vault after
  longterm-mem last wrote it
- WHEN sync would otherwise re-promote it
- THEN the page's content is left unmodified and a diagnostic names the
  skipped page

#### Scenario: Unmodified page updates normally

- GIVEN a promoted page that has not been locally modified since
  longterm-mem last wrote it
- WHEN sync re-promotes it
- THEN the normal update-in-place behavior applies with no skip

### Requirement: An Interrupted Promotion Leaves a Recoverable Vault

Traces to: longterm-mem R-029, R-030

No change-level `ID:` is claimed here on purpose. This rule was discovered
during delivery rather than specified up front, so it has no R-NNN of its
own; the longterm-mem space is otherwise contiguous and claiming a number
inside it would assert a provenance this requirement does not have.
`Traces to:` is the key that resolves a requirement, not `ID:`.

A promotion writes the page, its precedence-store entry, the master catalog
and the promotion log as separate durable steps, and no journal spans them.
A process killed between any two of them therefore always leaves one of them
ahead of the others, and the requirement is that whichever state that is, the
next run finishes the job instead of refusing it.

WHEN promotion writes a brand-new page, the promotion writer SHALL persist
that page's precedence-store entry BEFORE publishing the page, so that the
only ordering an interruption can leave is a recorded fingerprint with no
page — which the ordinary create path completes on the next run — and never a
published page with no recorded provenance, which the local-edit precedence
rule refuses.

IF a promoted page carries no precedence-store entry at all but its on-disk
content is byte-identical to what promotion would emit for that observation,
ignoring only the created and updated stamps taken from the wall clock, THEN
the promotion writer SHALL treat it as its own unrecorded write: adopt it into
the precedence store, refresh it, and repair its catalog and log registration,
rather than refusing it as unknown provenance. Any other divergence, in the
body or in any other frontmatter field, remains a refusal.

An interrupted UPDATE leaves the mirror-image state: the page holds a render
the precedence store never caught up with, so the entry still fingerprints
the previous one. Whenever the retry renders the same revision, the rule
above settles it. Whenever Engram has been revised in between, no comparison
against the incoming bytes can settle it at all — the two renders are of
different revisions and are supposed to differ — and the page is otherwise a
permanent refusal, because a refusal also suppresses the store write that
would have repaired it.

The precedence store SHALL therefore record, alongside each page's content
fingerprint, the Engram revision that fingerprinted render carried. IF a
promoted page's content no longer matches its entry, but the page's own
engram_revision stands above the revision that entry records and no higher
than the revision now being promoted, and the page's body still closes with
the promotion footer naming that observation at exactly the revision the page
claims, and the page's frontmatter is what promotion would render now, THEN
the promotion writer SHALL treat it as its own unrecorded update: republish
the page at the current revision and record the entry the interrupted run
never wrote.

The frontmatter comparison SHALL disregard exactly the lines two renders of
the writer's own may legitimately disagree on, and no others. Those are the
created and updated stamps; the engram_revision the two renders differ in by
construction; the status and related lines a status propagation rewrites in
place; and every line rendered from the observation that DESCRIBES it rather
than IDENTIFIES it — title, aliases, tags, engram_type, engram_sync_id and
project — which the observation itself moves when it is retitled, retyped,
merged into another project, or has a sync id backfilled between the
interrupted write and the retry. The observation-derived part of the disregarded
set SHALL be exactly the observation's descriptive fields, not a subset of
them.

The observation's IDENTIFYING field, engram_id, SHALL NOT be disregarded,
even though the observation supplies it too. It is the only frontmatter
witness of WHICH observation a page belongs to, so blanking it would make two
different observations' pages compare equal and let one observation's page
corroborate a render of another's. The comparison SHALL therefore compare
engram_id together with every key the writer alone emits — type, address and
sources — so that a key a human added, changed or deleted is a refusal.

Corroborating only the body's closing footer is not sufficient: a
frontmatter-only edit leaves that footer intact, and adopting such a page
overwrites the human's edit while reporting a successful update. Comparing
any DESCRIPTIVE observation-derived line is equally wrong in the other
direction: it refuses an ordinary retitle, retype, or a sync id backfilled
onto an observation that had none — each of which Engram can do between the
interrupted write and the retry, and the first of which this requirement's
own update scenario makes a first-class supported case — and that refusal is
a skip, so it suppresses the store write, the entry never advances, and every
later revision repeats it.

project is disregarded for consistency with the rest of the descriptive set,
not because a project move reaches this comparison: address allocation
matches an already-promoted page on project as well as engram_id, so an
observation moved to another project is allocated a FRESH address and never
reaches an in-place update at all. Blanking it is therefore correct and
harmless, but the wedge this rule exists to prevent is reached through the
fields that are not part of that lookup, engram_sync_id among them.

A page whose engram_revision is level with the revision its entry records has
been changed by someone other than the promotion writer — only the writer's
own renders advance that field — and remains a refusal.

An entry that records no revision at all is a vault promoted before the store
recorded one, and it carries NO evidence of the writer's own authorship. IF a
promoted page's content no longer matches an entry that records no revision,
THEN the promotion writer SHALL refuse that page. A moved frontmatter
fingerprint SHALL NOT be read as proof that one of the writer's own
unrecorded writes moved it: every render of the writer's own does move that
fingerprint, but so does a human editing any frontmatter line, so the
inference would unlock adoption on every legacy-tracked page a human has ever
hand-edited — no interruption required — and the adoption would overwrite
mid-body edits while reporting a successful update. Where the evidence is
ambiguous, this requirement's guarantee of preserving human edits SHALL win.

The fixed point such an entry would otherwise sit in SHALL be broken without
inference instead: an entry that records no revision whose page still matches
its fingerprints is not diverged at all, so the ordinary update path
republishes it and records the revision the older store never wrote. The
population of entries recording no revision therefore shrinks page by page,
with nothing adopted.

What remains is an entry recording no usable revision whose page has ALSO
diverged. That page is refused on every run, and no promotion of it can repair
the entry. It SHALL therefore be reported by the vault's own
precedence-sidecar diagnostic rather than left silent, so an operator learns
about a wedged page instead of the vault reporting healthy. "No usable
revision" SHALL mean the same thing in the diagnostic as in the adoption rule
— a recorded revision that is not positive, whether absent or negative —
so that no page the writer permanently refuses is left unreported.

A diverged page whose entry DOES record a positive revision is not reported by
that diagnostic: it is an ordinary local edit, refused and named by the
promotion writer itself on every run. That refusal is standing rather than
temporary — the page's engram_revision stays level with the entry's, so no
later revision adopts it — which is this requirement preserving the human's
edit, not a wedge to repair.

Two limits of this reconciliation are known and accepted, and both are stated
here rather than left to be rediscovered. An edit buried mid-body, or one
confined to any line the comparison has to disregard — created, updated,
engram_revision, status and related, plus the observation-derived title,
aliases, tags, engram_type, engram_sync_id and project — is indistinguishable
from the writer's own render, because the fingerprint that would have
separated them is exactly what the interruption destroyed. A human who
hand-sets a title or inserts an alias inside this window therefore loses that
edit to a reported update; keeping any of those lines compared would not
preserve the edit, it would only turn an ordinary Engram-side move into a
permanent refusal. In the other direction, a diverged page whose entry records no
revision is refused however well it otherwise corroborates, because nothing
distinguishes the writer's own unrecorded write from a human's edit there;
that refusal is reported, names the page, and is reported again by the
precedence-sidecar diagnostic, which is the side of the trade this
requirement takes.

#### Scenario: An interrupted create is finished by the next run

- GIVEN a create that persisted the page's precedence entry and was killed
  before the page itself was written
- WHEN promotion runs again for that observation
- THEN the page is written, registered in the catalog and the log, and the
  promotion reports a create

#### Scenario: A page left unrecorded by an older interrupted create is adopted

- GIVEN a promoted page whose precedence entry, catalog registration and log
  entry were all lost to an interruption, and whose content is what promotion
  would emit for its observation apart from the created and updated stamps
- WHEN promotion runs again for that observation
- THEN the page is adopted into the precedence store, its registration is
  repaired, and the promotion is not refused

#### Scenario: A genuinely edited untracked page is still refused

- GIVEN a promoted page with no precedence entry whose content differs from
  what promotion would emit by more than the created and updated stamps
- WHEN promotion runs again for that observation
- THEN the page is left byte-unchanged and the promotion is refused

#### Scenario: An interrupted update retried on a later day is reconciled

- GIVEN an update that published its page and was killed before its
  precedence entry was persisted, so the entry still fingerprints the
  previous render
- WHEN promotion runs again for that unchanged observation on a later day,
  so its retry differs from the page on disk in the created and updated
  stamps
- THEN the page is refreshed, its entry is recorded, and the promotion is not
  refused

#### Scenario: An interrupted update is reconciled after a further revision

- GIVEN that same interrupted update, and an observation Engram has since
  revised again, so the retry renders content the page never held
- WHEN promotion runs again for that observation
- THEN the page is republished at the current revision, its entry records
  that revision, and the promotion is not refused

#### Scenario: A page edited inside the update crash window is still refused

- GIVEN that same interrupted update, and a page a human has since edited so
  that its body no longer closes with the promotion footer for the revision
  it claims
- WHEN promotion runs again for that observation
- THEN the page is left byte-unchanged and the promotion is refused

#### Scenario: A frontmatter-only edit inside the update crash window is refused

- GIVEN that same interrupted update, and a page whose frontmatter a human has
  since edited — a key added, a value changed or a key deleted — leaving the
  body, promotion footer included, byte-identical
- WHEN promotion runs again for that observation
- THEN the page is left byte-unchanged and the promotion is refused

#### Scenario: A page level with the revision its entry records is refused

- GIVEN a page promoted completely at its current revision, whose body a human
  has since edited without disturbing the promotion footer or the frontmatter
- WHEN promotion runs again for a later revision of that observation
- THEN the page is left byte-unchanged and the promotion is refused

#### Scenario: A page claiming a revision Engram has not reached is refused

- GIVEN a page whose engram_revision, and whose promotion footer, name a
  revision above the one now being promoted
- WHEN promotion runs for that observation
- THEN the page is left byte-unchanged and the promotion is refused

#### Scenario: An interrupted update whose observation was retitled is reconciled

- GIVEN that same interrupted update, and an observation Engram has since
  revised AND retitled (or retyped), so the incoming render's
  observation-derived frontmatter differs from the page's
- WHEN promotion runs for that observation
- THEN the page is republished at the current revision, its entry records that
  revision, and the promotion is not refused

#### Scenario: An entry recording no revision stops wedging its undiverged page

- GIVEN a precedence entry carrying content fingerprints but no revision, and
  a page that still matches those fingerprints
- WHEN promotion runs for a further revision of that observation
- THEN the page is republished, its entry records the revision just published,
  and every later revision takes the ordinary update path

#### Scenario: An entry recording no revision refuses a diverged page

- GIVEN a precedence entry carrying content fingerprints but no revision, and
  a page whose content a human has since edited — in the body, in the
  frontmatter, or both
- WHEN promotion runs for a later revision of that observation
- THEN the page is left byte-unchanged and the promotion is refused,
  regardless of whether the entry's frontmatter fingerprint still matches

### Requirement: A Named Page Can Be Reconciled, One Address at a Time

Traces to: longterm-mem R-030

No change-level `ID:` is claimed here, for the reason given above.

The rule above leaves one residue on purpose: a promoted page whose entry
carries no evidence of the writer's own authorship — no entry at all, or one
recording no usable revision — and whose content has diverged. Promotion
refuses it, that refusal is a skip, and a skip suppresses the store write
that would have repaired the entry, so the page is refused identically on
every later run. The vault's precedence-sidecar diagnostic names exactly
those pages, and naming them is as far as an automatic path may go: nothing
in the bytes distinguishes the writer's own unrecorded write from a human's
edit.

A human naming ONE address supplies exactly the evidence that is missing.
The longterm-mem component SHALL therefore offer an explicit reconcile
operation that takes a single promoted page address, records that page's
current on-disk state as the writer's own last write, and leaves the page
byte-unchanged, so that a later promotion of it takes the ordinary update
path.

The operation SHALL refuse every bulk form: an all-addresses flag, and any
invocation naming more addresses than the operator wrote out. That refusal
is the design, not a limitation. The naming IS the consent the automatic
path lacks, and a bulk form would reintroduce behind a flag the silent
mass-adoption the ambiguity rules out, with the consent reduced to one
keystroke covering pages nobody inspected. The refusal SHALL be an explicit
error stating that exactly one address is expected, and SHALL leave the
precedence store unwritten.

The operation SHALL validate the supplied address against the promoted-page
address format before reading anything, and SHALL refuse an address that does
not match it. The address is operator-supplied and names both a file to read
and the key an entry is written under, so an unvalidated one reads a file
outside the promoted-pages directory and writes a sidecar key no promotion
ever looks up, which nothing removes. The operation SHALL further verify that
the file found at that address is that page: its own frontmatter address must
be the address named, and its project, when the page carries one, must be the
project named. A page that predates the project field SHALL NOT be refused for
lacking it.

The revision recorded SHALL be read from the page itself. A revision of ZERO
SHALL be adopted. Eligibility promotes a pinned or eligible-typed observation
whose revision count is zero, so zero is an ordinary promoted state reached by
the ordinary path, and it is exactly the state the precedence-sidecar
diagnostic names once such a page diverges — refusing it would leave the only
advertised repair declining the population it exists for. Adopting it does not
recreate the wedge: the wedged condition is an entry that no longer MATCHES
its page, and an adopted entry fingerprints the bytes on disk, so the next
promotion of that page takes the ordinary update path. A page whose own
revision cannot be read as an integer, or reads as a negative one, SHALL be
refused: that is not a value the writer's own renders produce, so there is no
revision to record.

Two states are not that residue, and SHALL NOT be adopted. An entry that
already fingerprints its page is a NO-OP reported as success, not an error:
the operation is run off a diagnostic report, a page repaired in between by
an ordinary promotion must not fail a command whose work is already done,
and an idempotent command can be re-run and scripted. An entry recording a
POSITIVE revision whose page has diverged is an ordinary local edit, and
SHALL be refused with the registration-conflict code: that page is not
wedged, the precedence rule is holding a human's edit, and promotion either
adopts the page on its own (when the page's own revision stands strictly above
the recorded one, which is the writer's own interrupted update) or names it on
every run. Adopting it here would overwrite that edit on the next promotion
while reporting an ordinary update. Being asked for it by name does not
change what would be destroyed.

An address with no promoted page SHALL fail with the not-found code, never
as a generic internal failure and never as a silent success.

#### Scenario: A wedged page leaves the wedged state

- GIVEN a promoted page whose precedence entry records no usable revision and
  whose content has diverged from that entry, so every promotion of it is
  refused
- WHEN an operator reconciles that one address by name
- AND a LATER revision of its observation is then promoted
- THEN that promotion is an ordinary update rather than a refusal, and the
  page is refreshed to the later revision

#### Scenario: A page promoted at revision zero leaves the wedged state

- GIVEN a promoted page whose observation has never been revised, so the page
  itself carries revision zero, and whose precedence entry has diverged from
  it — the state the precedence-sidecar diagnostic names
- WHEN an operator reconciles that address by name
- THEN the page is adopted at revision zero, the diagnostic no longer names
  it, and a LATER revision of its observation is promoted as an ordinary
  update rather than a refusal

#### Scenario: An address that is not a promoted-page address is refused

- GIVEN an address that does not match the promoted-page address format, such
  as one containing a path separator or a parent-directory segment
- WHEN reconcile is invoked with it
- THEN the invocation is refused before any file is read, and the precedence
  store is left unwritten

#### Scenario: A file that is not that page is refused

- GIVEN a file at the named address whose own frontmatter carries a different
  address, or a different project
- WHEN an operator reconciles that address by name
- THEN the adoption is refused and the precedence store is left unchanged

#### Scenario: A page with no entry at all is adopted

- GIVEN a promoted page the precedence store has no entry for
- WHEN an operator reconciles that address by name
- THEN the store records the page's current fingerprints and the revision the
  page itself carries

#### Scenario: Every bulk form is refused and writes nothing

- GIVEN a vault holding a page that could be reconciled
- WHEN reconcile is invoked with an all-addresses flag, with more than one
  address, or with no address at all
- THEN the invocation is refused with an error stating that exactly one
  address is expected, and the precedence store is left unwritten

#### Scenario: An ordinary local edit is refused rather than adopted

- GIVEN a promoted page whose entry records a positive revision and whose
  content a human has since edited
- WHEN an operator reconciles that address by name
- THEN the adoption is refused with the registration-conflict code and the
  precedence store is left unchanged, so the next promotion still preserves
  the edit

#### Scenario: An already-recorded page is a no-op, not a failure

- GIVEN a promoted page whose entry already fingerprints it
- WHEN an operator reconciles that address by name
- THEN nothing is written and the operation reports success, so running it
  off a stale diagnostic report is harmless

#### Scenario: An unknown address fails cleanly

- GIVEN an address with no promoted page in the vault
- WHEN an operator reconciles it
- THEN the operation fails with the not-found code

### Requirement: A Refused Promotion Is Reported as a Refusal

Traces to: longterm-mem R-030, R-032

No change-level `ID:` is claimed here, for the reason given above.

WHEN an explicit promote call is refused by the local-edit precedence rule,
the longterm-mem component SHALL report it as a refusal and SHALL exit with
the registration-conflict code, never with the success code and never with
wording that describes the observation as promoted — a page that was
deliberately not written is not a promotion, and a caller that cannot tell
the two apart cannot notice a vault that refuses the same page on every run.

#### Scenario: A refused promote is distinguishable from a successful one

- GIVEN a promoted page a human has edited directly in the vault
- WHEN an explicit promote call names its observation
- THEN the page is left byte-unchanged, the outcome is reported as a refusal
  naming the page, and the call exits with the registration-conflict code
  rather than the success code

### Requirement: Sync Promotes Unpromoted-or-Revised Observations

ID: R-009
Traces to: longterm-mem R-009

WHEN `sync` runs for project P, the longterm-mem component SHALL promote
every eligible observation for P that is either unpromoted or whose current
revision count exceeds the revision last promoted.

#### Scenario: Never-promoted eligible observation is promoted

- GIVEN an eligible, never-promoted observation for project P
- WHEN sync runs for P
- THEN it is promoted

#### Scenario: Revised eligible observation is re-promoted

- GIVEN an eligible observation already promoted at revision 2, now at
  revision 3
- WHEN sync runs
- THEN it is re-promoted

#### Scenario: Unchanged eligible observation is a no-op

- GIVEN an eligible observation already promoted at its current revision
  (unchanged)
- WHEN sync runs
- THEN it is not re-promoted

### Requirement: Sync Rebuilds the Index and Records Sync State

ID: R-031
Traces to: longterm-mem R-031

WHEN sync completes promoting eligible observations for project P, the
longterm-mem component SHALL rebuild P's vault index and record the sync
completion timestamp in that vault's own sync-state record.

#### Scenario: Index and sync-state both reflect the completed sync

- GIVEN a sync run that promoted three observations
- WHEN it completes
- THEN P's vault index reflects the new pages and the vault's sync-state
  record carries the completion timestamp

### Requirement: Soft-Delete and Supersession Propagate to the Vault

ID: R-033
Traces to: longterm-mem R-033

WHEN a promoted observation is soft-deleted or superseded in Engram, the
next sync SHALL patch that observation's vault page status field (to
`superseded`, or `archived` for a soft-delete with no successor) and its
related-links field to reference the successor's page — updating only those
frontmatter fields, never the page body, even on a locally edited page —
with the patch itself recorded in the precedence store so a later sync does
not misread it as a human edit.

#### Scenario: Supersession updates status and related, body untouched

- GIVEN a promoted observation later superseded by another already-promoted
  observation
- WHEN the next sync runs
- THEN its vault page's status becomes `superseded` and its related-links
  field points to the successor's page, with the page body unchanged

#### Scenario: Soft-delete with no successor archives the page

- GIVEN a promoted observation later soft-deleted with no successor relation
- WHEN the next sync runs
- THEN its vault page's status becomes `archived`

#### Scenario: Untouched observation keeps its status

- GIVEN a promoted observation with neither a soft-delete nor a supersession
  relation
- WHEN sync runs
- THEN its status remains unchanged

#### Scenario: Status patch on a locally edited page still lands (canon wins)

- GIVEN a promoted page a human or agent has edited locally, and its
  observation is now superseded in Engram
- WHEN the next sync runs
- THEN the status and related-links fields are patched despite the local
  edit, the page body is not rewritten, and the precedence store's recorded
  content is updated so a later sync does not treat this patch itself as a
  further human edit

### Requirement: Explicit Promote Surface

ID: R-032
Traces to: longterm-mem R-032

WHEN a user or agent explicitly names an Engram observation id for
promotion, the longterm-mem component SHALL promote that observation
through the same page-emission, addressing, and registration path used by
any other eligible observation, regardless of its automatic eligibility.

#### Scenario: Below-threshold observation is promoted via explicit call

- GIVEN an observation that is not pinned, not of an eligible type, and
  below the revision-count threshold
- WHEN an explicit promote call names it
- THEN it is promoted through the same page-emission, addressing, and
  registration path as any other eligible observation

#### Scenario: Invalid observation id is rejected

- GIVEN an invalid or nonexistent observation id
- WHEN an explicit promote call is made with it
- THEN the call is rejected with a clear error rather than silently doing
  nothing
