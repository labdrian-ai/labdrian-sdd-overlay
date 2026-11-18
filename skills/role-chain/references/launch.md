# Launch

- On Codex: pipe that brief to `labdrian review launch --runtime codex --feature F --base B --head H --rows R --model M`; it binds, runs the reviewer read-only and records it as `review record` does; never a native reviewer sub-agent.
- On OpenCode: send the brief without the diff; the plugin appends the frozen context (`labdrian review frozen-context`) and refuses a launch it cannot give one.
