// Package core is the part of the runtime lifecycle that needs nothing of the machine: the
// vocabulary (the Target a command names, the Action it asks for, the CapabilityStatus and the
// LifecycleResult an adapter answers with), the Adapter port every runtime implements, the
// Registry that maps a target to the Factory of its adapter, the Config the composition root
// hands those factories, and the rules that put a contract into a prompt or take it out.
//
// It is a domain package: it imports the pure standard library and the other domain packages
// (capability, contract), no file system, process or environment. The adapters of the runtimes
// (engine/runtime) implement the port and register themselves in a Registry the composition root
// builds; nothing is registered by being imported.
package core
