package main

import "os"

// main is the composition root of the engine command, and the one place that touches the
// process: it hands the arguments, the streams, the exit, the environment and the working
// directory of the process to the code that runs the subcommand, and that code reaches the machine
// through nothing else (deps.go; process_boundary_test.go keeps it so).
func main() {
	run(productionProcess(), productionDeps(), os.Args[1:])
}

// productionProcess is the process the program runs in.
func productionProcess() process {
	return process{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, exit: os.Exit, getwd: os.Getwd}
}

// productionDeps is the deps of the program: the environment of the process, resolved when a
// command asks for it.
func productionDeps() deps {
	return deps{getenv: os.Getenv, environ: os.Environ, userHomeDir: os.UserHomeDir}
}
