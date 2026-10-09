package main

// The cases of the read-back that fails or finds another text, and of the registry that changes kind
// around a lost write.

func propagateVerifyCases() []propagateCase {
	// readThenFail scripts n attempts that each read the old registry and then fail the read-back.
	readThenFail := func(w *propagateWorld, n int, readBack propagateRead) {
		for i := 0; i < n; i++ {
			w.script(registryFile, content(goldenRegistry), readBack)
		}
	}
	run := func(name, label string, n int, readBack propagateRead) propagateCase {
		return propagateCase{name, func(w *propagateWorld) {
			w.withContract()
			readThenFail(w, n, readBack)
			w.propagate(label, propagateArgs...)
		}}
	}
	return []propagateCase{
		run("propagate-loop-verify-read-fails", "the read-back fails on every attempt", 3, failure("input/output error")),
		run("propagate-loop-verify-read-absent", "the registry is gone at every read-back", 3, absent(registryFile)),
		run("propagate-loop-verify-read-fails-once", "the read-back fails once: the second attempt finds the block in place", 1, failure("input/output error")),
		run("propagate-loop-verify-read-differs-once", "the read-back is another text once: the second attempt finds the block in place", 1, content(foreignRegistry)),
		{"propagate-loop-clobbered-then-absent", func(w *propagateWorld) {
			w.withContract()
			w.script(registryFile, content(goldenRegistry), content(foreignRegistry), absent(registryFile), absent(registryFile))
			w.propagate("the write is lost, then the registry is absent twice: it existed, so the run fails", propagateArgs...)
		}},
		{"propagate-loop-clobbered-then-empty", func(w *propagateWorld) {
			w.withContract()
			w.script(registryFile, content(goldenRegistry), content(foreignRegistry), content(""), content(""))
			w.propagate("the write is lost, then the registry is empty twice: it existed, so the run fails", propagateArgs...)
		}},
		{"propagate-loop-clobbered-then-empty-then-absent", func(w *propagateWorld) {
			w.withContract()
			w.script(registryFile, content(goldenRegistry), content(foreignRegistry), content(""), absent(registryFile))
			w.propagate("the write is lost, then empty, then absent: the run fails", propagateArgs...)
		}},
		{"propagate-loop-absent-then-clobbered-every-time", func(w *propagateWorld) {
			w.withContract()
			w.script(registryFile, absent(registryFile),
				content(goldenRegistry), content(foreignRegistry),
				content(goldenRegistry), content(foreignRegistry))
			w.propagate("absent, then the write is lost twice: the run fails as not persisted", propagateArgs...)
		}},
		{"propagate-loop-empty-then-clobbered-then-ok", func(w *propagateWorld) {
			w.withContract()
			w.files[registryFile] = goldenRegistry
			w.script(registryFile, content(""))
			w.afterWrite[1] = clobber(foreignRegistry)
			w.propagate("empty, then the write is lost, then the third attempt writes", propagateArgs...)
		}},
	}
}
