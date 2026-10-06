// Package app holds the use cases of `engine skills` (Phase 9 unit H20): what each verb does,
// taking a typed input and the ports of the domain (engine/skills) it needs, and returning a
// typed result or an error that says what was refused. It reads no argument vector, prints
// nothing and exits nowhere: parsing the command line and telling a person what happened are the
// work of the CLI adapter in engine/cmd, which is the one place that knows the words of a
// terminal. A use case that is given the same input and ports gives the same result, whoever
// asks it.
package app
