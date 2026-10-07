package skills

// ApprovalBaselineEntry grandfathers one global skill that existed before the
// approval record did: its id and the SHA-256 of the exact SKILL.md bytes it
// had at the Phase 8 base commit.
type ApprovalBaselineEntry struct {
	ID     string
	SHA256 string
}

// approvalBaseline is the fixed list of global skills that predate the
// approval record, with the digest of each SKILL.md at the Phase 8 base
// (main at 53f545d). A baseline skill needs no record while its bytes still
// equal the pinned digest; the moment they differ, it needs a valid record
// like any other global skill. A skill outside this list always needs one.
//
// The list is data, sorted by id, and is never extended at run time. Adding an
// entry would exempt a skill from human approval, so a change here is a
// deliberate, reviewed decision; TestApprovalBaseline_IsWellFormedAndFixed
// pins the count and TestApprovalBaseline_PinnedToTheRepositoryRegistry pins
// the entries against the repository's real registry and SKILL.md files.
var approvalBaseline = []ApprovalBaselineEntry{
	{ID: "anti-generic-design", SHA256: "35d4285098f8c3e14e2055339205b013b9a205452b61d07305b747eed5754319"},
	{ID: "branch-pr", SHA256: "8553fdde3397c7dc1c2fda2b4ca848baf5d9185d0db65635b151755c4e627dc8"},
	{ID: "chained-pr", SHA256: "0a4a031f7b16879785d94ec771a51efc695f498f0d90adda66df35ea286102ef"},
	{ID: "chat-thread-analyzer", SHA256: "4206caae329c52e9618a19566be88b9848dd1295dd470137dd749574534965ee"},
	{ID: "cognitive-doc-design", SHA256: "80dcaeca45c35e830939ef5440ca270b147f6370e4afcb47cdab762097f8cde0"},
	{ID: "comment-writer", SHA256: "cb980612f7237b31a4afea87899275ba20f1f621ba237ecd4a3b440f0cf2e6da"},
	{ID: "gadu-operator", SHA256: "95ff2a81bf8f6949b2818449720ca71e90bdd8b60fb6b99c49b99d33c189d71a"},
	{ID: "gadu-orchestrate", SHA256: "e99ce1bb3e608079a7db2df23331347afd19749ec407edfba9df1b1003351033"},
	{ID: "gentle-ai-bench", SHA256: "49ac672887b4afe88b107de17edd8c4dff34bc4ada54903bcd45a68532dcc91e"},
	{ID: "go-testing", SHA256: "87205f2eff2f5dad905117e0fe7545ed432b728522ee742755fd25b22b612df3"},
	{ID: "inception-pipeline", SHA256: "5c230859849bddf9e11a819f4ece505c6fca6065f1c161f13e7f7215cd865b33"},
	{ID: "issue-creation", SHA256: "99beff12891c616f6949ddbfd5192f537f5b49c9692642e5f4cfbdb6c556f2b4"},
	{ID: "judgment-day", SHA256: "ac2a4fcfd762d8e3a873742f05a71287f96294f5063d740957f81b41ff8b8486"},
	{ID: "knowledge-ingestion", SHA256: "cd411d7cb9807c8cefdb045400ef1313f6a240600ed5cb564dd79695993c9775"},
	{ID: "prespec-malandra", SHA256: "71952728993594180b11630994e45785cab265863d355dbfce38d3461d8cb7a7"},
	{ID: "project-architect", SHA256: "8eaac3f295fba84a84a1051b5f85191493c0910b3e5779dcfa69867f7518626d"},
	{ID: "project-inception", SHA256: "02eb3dbfe92ce064c5e5a442c2f3f703753df409bdbbaf95774704840c530dd0"},
	{ID: "project-manifest", SHA256: "979f911e9d64b04a2410eb84c8225ffd4b84cc3031dce07badc5f724561c56c9"},
	{ID: "rdd-defect-workflow", SHA256: "7dcabfdf0aee1ddc9e509107b8b58b96ec6e0b9428732f7fe621001847e2a405"},
	{ID: "requirements-from-transcripts", SHA256: "7d8e8a4e68d24bd9ad18900a3670340d6cf612cadd78169f5c47d743aaa679e7"},
	{ID: "roadmap-maker", SHA256: "61b1076a6753b8566b678fd9b83cfbff5f0e0d8308206abd4d729e02e5930a9e"},
	{ID: "sdd-apply", SHA256: "4381d3eaccb097c29361cdb5450072c01ca9a1dd306b44d592b58eeea33d86ff"},
	{ID: "sdd-archive", SHA256: "c129a85389fd96e631ff10a0ea1c9f431d6e4e08e1d63fafc1cbc291bc48038f"},
	{ID: "sdd-design", SHA256: "1228f953145a59c2f9755023dc82986aa7cbcffda920cc673e379efe2c8aad8b"},
	{ID: "sdd-explore", SHA256: "6bc9385cbdbaf0da7515fb25967ad5835e85ec57534781e614b773c4fce23b0a"},
	{ID: "sdd-init", SHA256: "6de008c41fa2cdc6ba71dcd5ca8a7f7810c0b566c06f336d86573138e0917701"},
	{ID: "sdd-onboard", SHA256: "0e15efbe72add5da2bdffa016d97191216a21014dc7b5212d4984dddd85f65cd"},
	{ID: "sdd-propose", SHA256: "ea8b5d8c5ad3db0d94af32cbd628904224412daa716fb959025c2eb34945dd50"},
	{ID: "sdd-spec", SHA256: "1294f4eb31cfd6fde44978acbe10325f7995ccc9e1f8bed4b5a4c591c72043a3"},
	{ID: "sdd-tasks", SHA256: "6aacd0d65535542ccbf02865043d8fa17e977584a0b563e2a5b5af38ce3472f5"},
	{ID: "sdd-time-estimation", SHA256: "85719cb4b404aaaa0a18fbb8eccffe3414120833dd93aadf0a9273b7be50bc05"},
	{ID: "sdd-verify", SHA256: "03d2ead354dcd911dc392cb8199b985bc390ad7e97b275ef4289e3650ea35d6c"},
	{ID: "skill-creator", SHA256: "111ab06b93475ce87987f7727d66ee3d6ee5ba052638f579f80d9d1a81895d7e"},
	{ID: "skill-improver", SHA256: "2436353bd2a379b22a01400f77ffc53529506b13ca0f2750ed0033ecf0409738"},
	{ID: "skill-registry", SHA256: "abb0fb19c8a56d18cc29a0c9ccbabb39140c37b732a3a13511f23ec2d21c7664"},
	{ID: "systemic-issue-triage", SHA256: "d0562fa1e2f8cee55222a208878821936922e0ac5d8702c204ae53aa0963f014"},
	{ID: "work-unit-commits", SHA256: "f4aef73aaece708ea6fa76a1662cf1122474bfc1d4c98c005129fc81bd6e0c6c"},
}

// ApprovalBaseline returns a copy of the grandfathered baseline.
func ApprovalBaseline() []ApprovalBaselineEntry {
	out := make([]ApprovalBaselineEntry, len(approvalBaseline))
	copy(out, approvalBaseline)
	return out
}

// baselineDigest returns the pinned SKILL.md digest for id and whether id is in
// the baseline at all.
func baselineDigest(id string) (string, bool) {
	for _, e := range approvalBaseline {
		if e.ID == id {
			return e.SHA256, true
		}
	}
	return "", false
}

// BaselineLookup says whether a global skill is grandfathered, and with which SKILL.md digest.
// The verbs that judge approval ask it, so that a test can name its own baseline without touching
// the fixed one; the program asks FixedBaseline.
type BaselineLookup func(id string) (digest string, ok bool)

// FixedBaseline is the lookup of the baseline this package pins (ApprovalBaseline).
func FixedBaseline(id string) (string, bool) { return baselineDigest(id) }

// OrFixed is b, or the fixed baseline when b is nil.
func (b BaselineLookup) OrFixed() BaselineLookup {
	if b == nil {
		return FixedBaseline
	}
	return b
}
