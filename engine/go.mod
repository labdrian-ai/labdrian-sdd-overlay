module github.com/labdrian-ai/labdrian-sdd-overlay/engine

go 1.21

require (
	github.com/labdrian-ai/labdrian-sdd-overlay/archguard v0.0.0
	github.com/labdrian-ai/labdrian-sdd-overlay/identity v0.0.0
)

replace github.com/labdrian-ai/labdrian-sdd-overlay/archguard => ../archguard

replace github.com/labdrian-ai/labdrian-sdd-overlay/identity => ../identity
