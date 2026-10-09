// Package app holds the use cases of the session binding and of the projection hook (Phase 9
// unit H29): which workflow a repository follows, and what the Claude Code hook says about it.
//
// BindWorkflow is what 'workflow bind', 'workflow unbind' and 'workflow binding' do: judge a
// workflow, judge a binding that already exists, and bind, unbind or describe. HookService is what
// the projection hook does: read the binding of the repository and the workflow it names, ask the
// domain what to say (projection.Project) or whether to deny a call (projection.Gate), and, for a
// closed workflow, remove the binding that was read and only that one.
//
// Both take a typed request and the ports they need, and answer with a typed value or with a
// typed refusal. The ports are the domain's own (projection.RepoLocator and projection.BindingStore)
// and this package's (WorkflowReader and Clock); the adapters at the edge answer them
// (engine/gitfs, engine/projection/fsstore and the workflow log) and the composition root, engine/cmd,
// wires them. Nothing here reads an argument vector, a hook input or the environment, prints,
// exits, takes the time from the machine or opens a file: parsing the command line, decoding the
// hook's JSON, the words a person reads and the shape of the answer are the commands' work.
package app
