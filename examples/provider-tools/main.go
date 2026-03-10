package main

// This example shows how to use provider-defined tools like Anthropic's
// built-in web search. Provider tools are executed server-side by the model
// provider, so there's no local tool implementation needed.

import (
	"context"
	"fmt"
	"os"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
)

func main() {
	provider, err := anthropic.New(anthropic.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating provider:", err)
		os.Exit(1)
	}

	ctx := context.Background()

	model, err := provider.LanguageModel(ctx, "claude-sonnet-4-20250514")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating model:", err)
		os.Exit(1)
	}

	// Use the web search tool helper. Pass nil for defaults, or configure
	// with options like MaxUses, AllowedDomains, and UserLocation.
	webSearch := anthropic.WebSearchTool(nil)

	agent := fantasy.NewAgent(model,
		fantasy.WithProviderDefinedTools(webSearch),
	)

	result, err := agent.Generate(ctx, fantasy.AgentCall{
		Prompt: "What is the current weather in San Francisco?",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Println(result.Response.Content.Text())

	// Print any web sources the model cited.
	for _, source := range result.Response.Content.Sources() {
		fmt.Printf("Source: %s — %s\n", source.Title, source.URL)
	}
}
