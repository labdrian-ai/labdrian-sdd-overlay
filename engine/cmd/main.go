package main

import (
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability/presence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// main is the composition root of the engine command, and the one place that touches the
// process: it hands the arguments, the streams, the exit, the environment and the working
// directory of the process to the code that runs the subcommand, and that code reaches the machine
// through nothing else (deps.go; process_boundary_test.go keeps it so).
func main() {
	run(productionProcess(), productionDeps(), os.Args[1:])
}

// productionProcess is the process the program runs in.
func productionProcess() process {
	return process{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, exit: os.Exit}
}

// productionDeps is the deps of the program: the environment of the process, which a command reads
// when it needs a variable, and the facts main resolves from it once.
func productionDeps() deps {
	d := deps{
		getwd:       os.Getwd,
		getenv:      os.Getenv,
		environ:     os.Environ,
		userHomeDir: os.UserHomeDir,
		agentChild:  os.Getenv(agentChildVariable) == agentChildValue,
		// probeFS stays nil: the probe stats the operating system's files.
		probeTimeout: defaultProbeTimeout,
		openBindings: newBindingStore,
		gate:         projection.Gate,
	}
	// The prober looks at the home and PATH of the environment d gives, read when a verb asks.
	d.workflowProber = func() workflow.DependencyProber {
		home, path := runtimeProbeEnv(d)
		return presence.Prober{Home: home, Path: path}
	}
	return d
}
