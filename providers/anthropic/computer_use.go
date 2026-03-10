package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/fantasy"
	anthropicsdk "github.com/charmbracelet/anthropic-sdk-go"
	"github.com/charmbracelet/anthropic-sdk-go/option"
)

// computerUseToolID is the canonical identifier prefix for
// Anthropic computer use tools.
const computerUseToolID = "anthropic.computer_use"

// ComputerUseToolVersion identifies which version of the Anthropic
// computer use tool to use.
type ComputerUseToolVersion string

const (
	// ComputerUse20251124 selects the November 2024 version of the
	// computer use tool.
	ComputerUse20251124 ComputerUseToolVersion = "computer_20251124"
	// ComputerUse20250124 selects the January 2025 version of the
	// computer use tool.
	ComputerUse20250124 ComputerUseToolVersion = "computer_20250124"
)

// ComputerUseToolOptions holds the configuration for creating a
// computer use tool instance.
type ComputerUseToolOptions struct {
	// DisplayWidthPx is the width of the display in pixels.
	DisplayWidthPx int64
	// DisplayHeightPx is the height of the display in pixels.
	DisplayHeightPx int64
	// DisplayNumber is an optional X11 display number.
	DisplayNumber *int64
	// EnableZoom enables zoom support. Only used with the
	// ComputerUse20251124 version.
	EnableZoom *bool
	// ToolVersion selects which computer use tool version to use.
	ToolVersion ComputerUseToolVersion
	// CacheControl sets optional cache control for the tool.
	CacheControl *CacheControl
}

// NewComputerUseTool creates a new provider-defined tool configured
// for Anthropic computer use. The returned tool can be passed
// directly into a fantasy tool set.
func NewComputerUseTool(opts ComputerUseToolOptions) fantasy.ProviderDefinedTool {
	args := map[string]any{
		"display_width_px":  opts.DisplayWidthPx,
		"display_height_px": opts.DisplayHeightPx,
		"tool_version":      string(opts.ToolVersion),
	}
	if opts.DisplayNumber != nil {
		args["display_number"] = *opts.DisplayNumber
	}
	if opts.EnableZoom != nil {
		args["enable_zoom"] = *opts.EnableZoom
	}
	if opts.CacheControl != nil {
		args["cache_control"] = *opts.CacheControl
	}
	return fantasy.ProviderDefinedTool{
		ID:   computerUseToolID,
		Name: "computer",
		Args: args,
	}
}

// IsComputerUseTool reports whether tool is an Anthropic computer
// use tool. It checks for a ProviderDefinedTool whose ID starts
// with the computer use tool prefix.
func IsComputerUseTool(tool fantasy.Tool) bool {
	pdt, ok := tool.(fantasy.ProviderDefinedTool)
	if !ok {
		return false
	}
	return strings.HasPrefix(pdt.ID, computerUseToolID)
}

// getComputerUseVersion extracts the ComputerUseToolVersion from a
// provider-defined tool's Args map. It returns the version and true
// if present, or the zero value and false otherwise.
func getComputerUseVersion(tool fantasy.ProviderDefinedTool) (ComputerUseToolVersion, bool) {
	v, ok := tool.Args["tool_version"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return ComputerUseToolVersion(s), true
}

// needsBetaAPI reports whether any tool in the slice is a computer
// use tool, which requires the Anthropic beta API.
func needsBetaAPI(tools []fantasy.Tool) bool {
	for _, t := range tools {
		if IsComputerUseTool(t) {
			return true
		}
	}
	return false
}

// betaFlagForVersion returns the Anthropic beta header value for
// the given computer use tool version.
func betaFlagForVersion(version ComputerUseToolVersion) (string, error) {
	switch version {
	case ComputerUse20251124:
		return "computer-use-2025-11-24", nil
	case ComputerUse20250124:
		return anthropicsdk.AnthropicBetaComputerUse2025_01_24, nil
	default:
		return "", fmt.Errorf(
			"unsupported computer use tool version: %q", version,
		)
	}
}

// detectComputerUseVersion scans tools for computer use tools and
// returns their version. If multiple computer use tools are present
// they must all share the same version; otherwise an error is
// returned. If no computer use tools are found it returns ("", nil).
func detectComputerUseVersion(tools []fantasy.Tool) (ComputerUseToolVersion, error) {
	var found ComputerUseToolVersion
	var seen bool

	for _, t := range tools {
		pdt, ok := t.(fantasy.ProviderDefinedTool)
		if !ok || !strings.HasPrefix(pdt.ID, computerUseToolID) {
			continue
		}

		version, ok := getComputerUseVersion(pdt)
		if !ok {
			continue
		}

		if !seen {
			found = version
			seen = true
			continue
		}

		if version != found {
			return "", fmt.Errorf(
				"conflicting computer use tool versions: %q and %q",
				found, version,
			)
		}
	}

	return found, nil
}

// computerUseRequestOptions builds the request options needed to
// send computer use tools through the regular Messages API. It
// adds the beta query parameter and header, then serializes the
// existing params.Tools together with the computer use tools into
// a single JSON array injected via WithJSONSet.
//
// This avoids duplicating the entire beta API surface: the wire
// format is identical, only the URL query and a header differ.
func computerUseRequestOptions(tools []fantasy.Tool, params *anthropicsdk.MessageNewParams) ([]option.RequestOption, error) {
	version, err := detectComputerUseVersion(tools)
	if err != nil {
		return nil, err
	}
	betaFlag, err := betaFlagForVersion(version)
	if err != nil {
		return nil, err
	}

	// Marshal the regular tools that prepareParams already placed
	// in params.Tools, then append the computer use tool JSON
	// objects so they all go out in a single "tools" array.
	var merged []json.RawMessage
	for _, t := range params.Tools {
		b, err := json.Marshal(t)
		if err != nil {
			return nil, fmt.Errorf("marshal tool: %w", err)
		}
		merged = append(merged, b)
	}
	for _, t := range tools {
		pdt, ok := t.(fantasy.ProviderDefinedTool)
		if !ok || !strings.HasPrefix(pdt.ID, computerUseToolID) {
			continue
		}
		cu, err := computerUseToolJSON(pdt)
		if err != nil {
			return nil, err
		}
		merged = append(merged, cu)
	}

	return []option.RequestOption{
		option.WithQuery("beta", "true"),
		option.WithHeaderAdd("anthropic-beta", betaFlag),
		option.WithJSONSet("tools", merged),
	}, nil
}

// computerUseToolJSON builds the JSON representation of a computer
// use tool from a ProviderDefinedTool's Args.
func computerUseToolJSON(pdt fantasy.ProviderDefinedTool) (json.RawMessage, error) {
	version, ok := getComputerUseVersion(pdt)
	if !ok {
		return nil, fmt.Errorf("computer use tool missing version")
	}

	tool := map[string]any{
		"type": string(version),
		"name": "computer",
	}

	// Copy dimension fields. Args values may be int64 (direct
	// construction) or float64 (after JSON round-trip).
	tool["display_width_px"] = toInt64(pdt.Args["display_width_px"])
	tool["display_height_px"] = toInt64(pdt.Args["display_height_px"])

	if v, ok := pdt.Args["display_number"]; ok {
		tool["display_number"] = toInt64(v)
	}
	if v, ok := pdt.Args["enable_zoom"]; ok {
		tool["enable_zoom"] = v
	}
	if _, ok := pdt.Args["cache_control"]; ok {
		tool["cache_control"] = map[string]string{"type": "ephemeral"}
	}

	return json.Marshal(tool)
}

// toInt64 converts a numeric value that may be int64 or float64
// (the latter from JSON round-tripping) to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}
