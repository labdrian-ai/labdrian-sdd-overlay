package main

import "errors"

// The cases of a registry that is absent and empty in turn, and of the runs that fail before the
// loop can help.

func propagateMixedReadCases() []propagateCase {
	run := func(name, label string, answers ...propagateRead) propagateCase {
		return propagateCase{name, func(w *propagateWorld) {
			w.withContract()
			w.script(registryFile, answers...)
			w.propagate(label, propagateArgs...)
		}}
	}
	return []propagateCase{
		run("propagate-loop-mixed-absent-empty-empty", "absent, then empty twice: the registry exists, so the run fails",
			absent(registryFile), content(""), content("")),
		run("propagate-loop-mixed-empty-absent-absent", "empty, then absent twice: the registry existed, so the run fails",
			content(""), absent(registryFile), absent(registryFile)),
		run("propagate-loop-mixed-empty-absent-empty", "empty, absent, empty: the registry existed, so the run fails",
			content(""), absent(registryFile), content("")),
		run("propagate-loop-mixed-absent-absent-empty", "absent twice, then empty: the registry exists, so the run fails",
			absent(registryFile), absent(registryFile), content("")),
		run("propagate-loop-mixed-empty-empty-absent", "empty twice, then absent: the registry existed, so the run fails",
			content(""), content(""), absent(registryFile)),
	}
}

func propagateFailureCases() []propagateCase {
	return []propagateCase{
		{"propagate-loop-refuses-what-it-cannot-do", func(w *propagateWorld) {
			w.withContract()
			w.files[registryFile] = goldenRegistry
			w.propagate("no registry flag", "--contract-file", contractFile)
			w.propagate("no contract source", "--registry", registryFile)
			w.propagate("an unknown embedded contract", "--registry", registryFile, "--embedded-contract", "no-such-contract")
			w.propagate("a contract file that is missing", "--registry", registryFile, "--contract-file", "/virtual/missing.md")
		}},
		{"propagate-loop-contract-failures", func(w *propagateWorld) {
			w.withContract()
			w.files[registryFile] = goldenRegistry
			w.script(contractFile, failure("permission denied"))
			w.propagate("the contract cannot be read", propagateArgs...)
			w.files[contractFile] = "no frontmatter here"
			w.propagate("the contract has no frontmatter", propagateArgs...)
			w.files[contractFile] = contractDoc("applies_to_phases: []")
			w.propagate("the contract applies to no phase", propagateArgs...)
		}},
		{"propagate-loop-registry-unreadable", func(w *propagateWorld) {
			w.withContract()
			w.script(registryFile, failure("permission denied"))
			w.propagate("the registry cannot be read: no retry", propagateArgs...)
		}},
		{"propagate-loop-unreadable-after-a-torn-read", func(w *propagateWorld) {
			w.withContract()
			w.script(registryFile, content(""), failure("input/output error"))
			w.propagate("empty, then unreadable: the failure is forwarded as it is", propagateArgs...)
		}},
		{"propagate-loop-contract-lost-after-a-torn-read", func(w *propagateWorld) {
			w.withContract()
			w.files[registryFile] = goldenRegistry
			w.script(registryFile, content(""))
			w.script(contractFile, content(w.files[contractFile]), failure("permission denied"))
			w.propagate("empty, then the contract cannot be read: forwarded, no more retries", propagateArgs...)
		}},
		{"propagate-loop-write-fails", func(w *propagateWorld) {
			w.withContract()
			w.files[registryFile] = goldenRegistry
			w.writeErrors[1] = errors.New("no space left on device")
			w.propagate("the write fails: forwarded, no retry", propagateArgs...)
		}},
	}
}
