package main

import "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"

// embedClientFactory builds the client of the embedding backend a command talks to. It has the shape of
// embed.NewClient, which is what the composition root hands the commands; a test hands one that points the
// client at a backend of its own, so that no test reaches the embedding server of whoever runs it.
type embedClientFactory func(embed.Config) (*embed.Client, error)

// commands are the subcommands, wired to what lives outside the process. main builds them once, with
// productionCommands; nothing else in the program names the real embedding backend.
type commands struct {
	// newEmbedClient builds the embedding client of `index --embeddings`, of `query` (and the top-up of its
	// index), of `doctor`'s backend probe and of the MCP session.
	newEmbedClient embedClientFactory
}

// productionCommands are the commands as the binary runs them: they talk to the embedding backend at the
// address its configuration names, or at embed.DefaultEndpoint when it names none.
func productionCommands() commands {
	return commands{newEmbedClient: embed.NewClient}
}
