package main

import (
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// deployRefVariable names the ref `pipkg check` compares a package against when the checkout is
// not main (CI on a pull request, a feature-branch shelltest).
const deployRefVariable = "LABDRIAN_PI_DEPLOY_REF"

// pipkgOptionsFromEnv reads, once, the environment pipkg depends on and hands it down as a value:
// the package reads no variable of its own.
func pipkgOptionsFromEnv(getenv func(string) string) pipkg.Options {
	return pipkg.Options{DeployRef: strings.TrimSpace(getenv(deployRefVariable))}
}
