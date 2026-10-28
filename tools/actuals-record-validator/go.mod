module github.com/labdrian-ai/labdrian-sdd-overlay/tools/actuals-record-validator

go 1.21

require (
	github.com/labdrian-ai/labdrian-sdd-overlay/engine v0.0.0
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
)

require golang.org/x/text v0.14.0 // indirect

replace github.com/labdrian-ai/labdrian-sdd-overlay/engine => ../../engine
