package main

// The cases of the registry that is absent or empty, one kind of read on every attempt or a read that
// recovers. The other cases of the loop are in propagate_golden_failure_cases_test.go (reads that
// change kind, the runs that fail), propagate_golden_write_cases_test.go (writes, a foreign writer
// that undoes one) and propagate_golden_verify_cases_test.go (the read-back); the driver and the
// scripting helpers (failure, writeErrors, afterWrite) are in propagate_golden_test.go.

func propagateAbsentCases() []propagateCase {
	return []propagateCase{
		{"propagate-loop-absent", func(w *propagateWorld) {
			w.withContract()
			w.propagate("no registry: three reads, then a clean no-op", propagateArgs...)
			w.propagate("no registry, one is required: the first read fails, no retry", append(propagateArgs, "--require-registry")...)
		}},
		{"propagate-loop-absent-then-present", func(w *propagateWorld) {
			w.withContract()
			w.files[registryFile] = goldenRegistry
			w.script(registryFile, absent(registryFile))
			w.propagate("absent once, then the registry is there: the second attempt writes", propagateArgs...)
		}},
		{"propagate-loop-absent-twice-then-present", func(w *propagateWorld) {
			w.withContract()
			w.files[registryFile] = goldenRegistry
			w.script(registryFile, absent(registryFile), absent(registryFile))
			w.propagate("absent twice, then the registry is there: the third attempt writes", propagateArgs...)
		}},
	}
}

func propagateEmptyCases() []propagateCase {
	return []propagateCase{
		{"propagate-loop-empty", func(w *propagateWorld) {
			w.withContract()
			w.files[registryFile] = ""
			w.propagate("empty on every attempt", propagateArgs...)
			w.files[registryFile] = " \n\t\n"
			w.propagate("white space on every attempt", propagateArgs...)
			w.files[registryFile] = ""
			w.propagate("empty, with the registry required", append(propagateArgs, "--require-registry")...)
		}},
		{"propagate-loop-empty-then-present", func(w *propagateWorld) {
			w.withContract()
			w.files[registryFile] = goldenRegistry
			w.script(registryFile, content(""))
			w.propagate("empty once, then the registry is whole: the second attempt writes", propagateArgs...)
		}},
		{"propagate-loop-empty-twice-then-present", func(w *propagateWorld) {
			w.withContract()
			w.files[registryFile] = goldenRegistry
			w.script(registryFile, content(""), content(" \n"))
			w.propagate("empty twice, then the registry is whole: the third attempt writes", propagateArgs...)
		}},
	}
}
