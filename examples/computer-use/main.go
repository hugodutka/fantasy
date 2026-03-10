package main

// This is a minimal example showing the API plumbing for Anthropic's computer
// use tool. It demonstrates how to wire up the provider, model, and tool, then
// inspect the tool calls that Claude returns.
//
// In a real implementation the caller would execute each action (screenshot,
// click, type, etc.) inside a sandboxed environment (VM, container, or VNC
// session) and feed the results back. The loop would continue with tool results
// until Claude signals it is done.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
)

func main() {
	// Set up the Anthropic provider.
	provider, err := anthropic.New(anthropic.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")))
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not create provider:", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Pick the model.
	model, err := provider.LanguageModel(ctx, "claude-opus-4-6")
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not get language model:", err)
		os.Exit(1)
	}

	// Create a computer use tool. This tells Claude the dimensions of the
	// virtual display it will be controlling.
	computerTool := anthropic.NewComputerUseTool(anthropic.ComputerUseToolOptions{
		DisplayWidthPx:  1920,
		DisplayHeightPx: 1080,
		ToolVersion:     anthropic.ComputerUse20251124,
	})

	// Build a Call with a simple prompt and the computer use tool.
	call := fantasy.Call{
		Prompt: fantasy.Prompt{
			fantasy.NewUserMessage("Take a screenshot of the desktop"),
		},
		Tools: []fantasy.Tool{computerTool},
	}

	// Ask the model to generate a response.
	resp, err := model.Generate(ctx, call)
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate failed:", err)
		os.Exit(1)
	}

	// Inspect the response content. Claude will typically reply with one
	// or more tool calls describing the actions it wants to perform.
	for _, tc := range resp.Content.ToolCalls() {
		fmt.Printf("Tool call: %s (id=%s)\n", tc.ToolName, tc.ToolCallID)

		// The Input field is a JSON string describing the requested
		// action (e.g. {"action": "screenshot"} or
		// {"action": "click", "coordinate": [100, 200]}).
		var action map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &action); err != nil {
			fmt.Fprintln(os.Stderr, "could not parse tool input:", err)
			os.Exit(1)
		}
		fmt.Printf("  Action: %v\n", action)

		// In a real agent loop you would:
		//  1. Execute the action in a sandboxed environment.
		//  2. Capture the result (e.g. a screenshot as a base64 image).
		//  3. Build a new Call that includes the tool result and send it
		//     back to model.Generate.
		//  4. Repeat until Claude stops requesting tool calls.
		fmt.Println("  -> (stub) would execute action and return screenshot")
	}

	// Print any text content Claude included alongside the tool calls.
	if text := resp.Content.Text(); text != "" {
		fmt.Println("\nClaude said:", text)
	}
}
