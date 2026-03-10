# Computer Use Tool Support

## Overview

Add support for Anthropic's computer use tool to Fantasy. Computer use is
a beta Anthropic feature that lets Claude interact with desktop environments
via screenshot capture, mouse control, and keyboard input. The tool is
schema-less (schema is built into Claude's model) and requires the beta
messages API, a beta header, and the computer use tool JSON injected into
the request body.

## Background

### How Computer Use Works

1. The caller registers a computer use tool with display dimensions.
2. Claude returns `tool_use` blocks with actions like `screenshot`,
   `left_click`, `type`, `key`, `mouse_move`, `scroll`, `zoom`, etc.
3. The caller executes the action in a sandboxed environment and returns
   a `tool_result` (typically a screenshot image).
4. The loop repeats until Claude is done.

### API Requirements

- Computer use requires the **beta** messages endpoint. The wire format
  is identical to the regular Messages API; only the URL query parameter
  (`?beta=true`) and an `anthropic-beta` header differ.
- A beta header must be sent: `"computer-use-2025-11-24"` for
  `computer_20251124`, `"computer-use-2025-01-24"` for
  `computer_20250124`.
- Computer use tools are serialized as raw JSON and injected into the
  request body alongside regular `ToolUnionParam` entries via SDK
  request options.

### Tool Versions

| Tool Version         | Models                                    | Beta Flag                    |
|----------------------|-------------------------------------------|------------------------------|
| `computer_20251124`  | Claude Opus 4.6, Sonnet 4.6, Opus 4.5    | `computer-use-2025-11-24`    |
| `computer_20250124`  | Sonnet 4.5, Haiku 4.5, Opus 4.1, etc.    | `computer-use-2025-01-24`    |

### SDK Constants Used (charmbracelet/anthropic-sdk-go)

- `AnthropicBetaComputerUse2025_01_24` — beta flag for v20250124
- No SDK constant exists for v20251124; the raw string
  `"computer-use-2025-11-24"` is used directly.

## Architecture Decision

The computer use tool is an Anthropic-specific, schema-less, beta-only
tool. Fantasy already has a `ProviderDefinedTool` type in its core
(`content.go`) with `ID`, `Name`, and `Args` fields. The implementation:

1. Exposes a user-facing constructor in `providers/anthropic/` that
   creates a `fantasy.ProviderDefinedTool` with the right ID/args.
2. In the Anthropic provider's `toTools()`, detects computer use
   `ProviderDefinedTool` entries and **skips** them (they are handled
   separately).
3. When `needsBetaAPI()` detects a computer use tool, builds SDK
   request options (`option.WithQuery`, `option.WithHeaderAdd`,
   `option.WithJSONSet`) that add the beta query/header and inject
   the merged tools JSON into the request body.
4. Passes these request options to the **regular** `client.Messages.New()`
   / `client.Messages.NewStreaming()` — no separate beta client path.

This avoids duplicating the entire beta API type surface (`Beta*` SDK
types). The regular and beta wire formats are identical; only the URL
query parameter and a header differ.

## TODOs

### Phase 1: Provider-Defined Tool Registration

- [x] Add `ComputerUseToolVersion` type and constants
      (`ComputerUse20251124`, `ComputerUse20250124`) in
      `providers/anthropic/computer_use.go`.
- [x] Add `ComputerUseToolOptions` struct with `DisplayWidthPx`,
      `DisplayHeightPx`, `DisplayNumber`, `EnableZoom`,
      `ToolVersion`, `CacheControl` fields.
- [x] Add `NewComputerUseTool(opts ComputerUseToolOptions)` constructor
      that returns a `fantasy.ProviderDefinedTool` with
      `ID: "anthropic.computer_use"` and the options serialized into
      `Args`.
- [x] Add helper `IsComputerUseTool(tool fantasy.Tool) bool` that checks
      the `ProviderDefinedTool.ID` prefix.

### Phase 2: Beta API Plumbing via Request Options

- [x] Add `needsBetaAPI(tools []fantasy.Tool) bool` helper that scans
      for computer use tools.
- [x] Add `betaFlagForVersion(version) (string, error)` that maps a
      `ComputerUseToolVersion` to the correct `anthropic-beta` header
      value.
- [x] Add `detectComputerUseVersion(tools) (ComputerUseToolVersion, error)`
      that scans tools, returns the version if all computer use tools
      agree, or an error on version conflict.
- [x] Add `computerUseToolJSON(pdt) (json.RawMessage, error)` that
      serializes a `ProviderDefinedTool` to the Anthropic computer use
      tool JSON format (handles both int64 and float64 numeric values
      for JSON round-trip safety).
- [x] Add `computerUseRequestOptions(tools, params) ([]option.RequestOption, error)`
      that builds three SDK request options:
      - `option.WithQuery("beta", "true")` — beta URL parameter
      - `option.WithHeaderAdd("anthropic-beta", betaFlag)` — beta header
      - `option.WithJSONSet("tools", merged)` — merged tools array
        containing both the regular `params.Tools` and the computer use
        tool JSON.
- [x] In `toTools()`, skip computer use `ProviderDefinedTool` entries
      (they are handled by the request options path).

### Phase 3: Generate and Stream with Request Options

- [x] Update `Generate()` to call `computerUseRequestOptions()` when
      `needsBetaAPI()` is true, and pass the resulting options to
      `client.Messages.New()`.
- [x] Update `Stream()` with the same pattern for
      `client.Messages.NewStreaming()`.
- [x] Response parsing is unchanged — the regular `Message` and
      `MessageStreamEventUnion` types parse beta responses correctly
      since the JSON wire format is identical.

### Phase 4: Provider Options

- [x] `ComputerUseToolOptions` supports `CacheControl` (using the
      existing `CacheControl` type in the provider).

### Phase 5: Unit Tests

- [x] `TestNewComputerUseTool` — verifies the returned
      `ProviderDefinedTool` has the correct ID and Args.
- [x] `TestIsComputerUseTool` — verifies detection for computer use
      tools, function tools, and other provider-defined tools.
- [x] `TestNeedsBetaAPI` — verifies with empty, function-only, and
      mixed tool sets.
- [x] `TestDetectComputerUseVersion` — verifies single version, no
      tools, matching versions, and conflicting versions.
- [x] `TestComputerUseToolJSON` — verifies JSON output for v20250124,
      v20251124 with enable_zoom, and direct int64 args.
- [x] `TestComputerUseRequestOptions` — verifies option count for
      single computer use tool and merged function + computer use tools.
- [x] `TestGenerate_BetaAPI` — mock HTTP server validates beta header
      is sent; verifies tool_use response parsing.
- [x] `TestStream_BetaAPI` — mock HTTP server with SSE responses;
      verifies streaming with beta header.

### Phase 6: Integration / Provider Tests

- [x] VCR-based integration test `TestAnthropicComputerUse` in
      `providertests/anthropic_test.go` with two subtests:
      - `computer_use` — non-streaming: send prompt, receive screenshot
        tool call, return screenshot image, receive text.
      - `computer_use_streaming` — streaming variant of the same flow.
- [x] VCR cassettes in
      `providertests/testdata/TestAnthropicComputerUse/claude-sonnet-4/`.

### Phase 7: Example

- [x] `examples/computer-use/main.go` — minimal computer use agent
      loop: configure the Anthropic provider, register the computer use
      tool, handle `screenshot` / `left_click` / `type` actions with
      stub implementations, and run the agent.

### Phase 8: Documentation

- [x] `README.md` updated: "Provider tools (partial: Anthropic computer
      use supported; e.g. web_search not yet)".
- [x] Doc comments on all exported types and functions.

## Out of Scope

- Implementing the actual screen capture, mouse, and keyboard execution
  (that is the caller's responsibility — Fantasy provides the API
  plumbing).
- The `text_editor` and `bash` beta tools (these can follow the same
  pattern in a future PR).
- Coordinate scaling helpers (useful but not part of the core
  integration).
- `GenerateObject` / `StreamObject` error-guarding for computer use
  tools (not implemented; computer use tools are silently ignored in
  object generation since `toTools()` skips them).

## Key Files

| File | Purpose |
|------|---------|
| `providers/anthropic/computer_use.go` | Tool constructors, version types, request option helpers (`computerUseRequestOptions`, `computerUseToolJSON`, `toInt64`). |
| `providers/anthropic/anthropic.go` | `Generate()` and `Stream()` call `computerUseRequestOptions()` when `needsBetaAPI()` is true. `toTools()` skips computer use entries. |
| `providers/anthropic/provider_options.go` | Unchanged — no computer-use-specific fields. `CacheControl` type reused by `ComputerUseToolOptions`. |
| `providers/anthropic/anthropic_test.go` | Unit tests for all computer use helpers and beta API integration. |
| `providertests/anthropic_test.go` | VCR integration tests (`TestAnthropicComputerUse`). |
| `examples/computer-use/main.go` | Example usage. |
