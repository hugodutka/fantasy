package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/responses"
)

// computerUseToolID is the canonical identifier for OpenAI computer use tools.
const computerUseToolID = "openai.computer_use"

// TypeComputerUseMetadata is the type identifier for OpenAI computer use metadata.
const TypeComputerUseMetadata = Name + ".computer_use_metadata"

// ComputerUseMetadata represents metadata returned by an OpenAI computer use tool call.
type ComputerUseMetadata struct {
	// RawJSON holds the raw JSON payload from the computer use tool call.
	RawJSON string `json:"raw_json"`
}

// Options implements the ProviderOptionsData interface.
func (*ComputerUseMetadata) Options() {}

// MarshalJSON implements custom JSON marshaling with type info for ComputerUseMetadata.
func (m ComputerUseMetadata) MarshalJSON() ([]byte, error) {
	type plain ComputerUseMetadata
	return fantasy.MarshalProviderType(TypeComputerUseMetadata, plain(m))
}

// UnmarshalJSON implements custom JSON unmarshaling with type info for ComputerUseMetadata.
func (m *ComputerUseMetadata) UnmarshalJSON(data []byte) error {
	type plain ComputerUseMetadata
	var p plain
	if err := fantasy.UnmarshalProviderType(data, &p); err != nil {
		return err
	}
	*m = ComputerUseMetadata(p)
	return nil
}

// NewComputerUseTool creates a new provider-defined tool configured
// for OpenAI computer use. The returned tool can be passed directly
// into a fantasy tool set.
func NewComputerUseTool() fantasy.ProviderDefinedTool {
	return fantasy.ProviderDefinedTool{
		ID:   computerUseToolID,
		Name: "computer",
	}
}

// IsComputerUseTool reports whether tool is an OpenAI computer use tool.
// It checks for a ProviderDefinedTool whose ID starts with the
// computer use tool prefix.
func IsComputerUseTool(tool fantasy.Tool) bool {
	pdt, ok := tool.(fantasy.ProviderDefinedTool)
	if !ok {
		return false
	}
	return strings.HasPrefix(pdt.ID, computerUseToolID)
}

// hasComputerUseTool reports whether any tool in the slice is a
// computer use tool.
func hasComputerUseTool(tools []fantasy.Tool) bool {
	for _, t := range tools {
		if IsComputerUseTool(t) {
			return true
		}
	}
	return false
}

// parseComputerCall extracts the call_id and action data from the
// raw JSON of a computer_call output item. It prefers the batched
// "actions" field over the singular "action" field.
func parseComputerCall(rawJSON string) (callID string, input string, err error) {
	var raw struct {
		CallID  string          `json:"call_id"`
		Actions json.RawMessage `json:"actions"`
		Action  json.RawMessage `json:"action"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return "", "", fmt.Errorf("parse computer_call: %w", err)
	}
	callID = raw.CallID

	actionData := raw.Actions
	if len(actionData) == 0 {
		actionData = raw.Action
	}
	if len(actionData) == 0 {
		return callID, "{}", nil
	}
	return callID, string(actionData), nil
}

// computerUseTools marshals the existing params.Tools together with
// a {"type":"computer"} entry and returns request options that
// inject the merged array via WithJSONSet.
func computerUseTools(params *responses.ResponseNewParams) []option.RequestOption {
	var merged []json.RawMessage
	for _, t := range params.Tools {
		b, err := json.Marshal(t)
		if err != nil {
			continue
		}
		merged = append(merged, b)
	}
	merged = append(merged, json.RawMessage(`{"type":"computer"}`))
	return []option.RequestOption{
		option.WithJSONSet("tools", merged),
	}
}

// GetComputerUseMetadata extracts computer use metadata from provider
// options.
func GetComputerUseMetadata(providerOptions fantasy.ProviderOptions) *ComputerUseMetadata {
	if opts, ok := providerOptions[Name]; ok {
		if meta, ok := opts.(*ComputerUseMetadata); ok {
			return meta
		}
	}
	return nil
}

// ComputerActionType identifies the kind of action requested by the
// model in a computer use tool call.
type ComputerActionType string

const (
	ComputerActionClick       ComputerActionType = "click"
	ComputerActionDoubleClick ComputerActionType = "double_click"
	ComputerActionDrag        ComputerActionType = "drag"
	ComputerActionKeypress    ComputerActionType = "keypress"
	ComputerActionMove        ComputerActionType = "move"
	ComputerActionScreenshot  ComputerActionType = "screenshot"
	ComputerActionScroll      ComputerActionType = "scroll"
	ComputerActionTyping      ComputerActionType = "type"
	ComputerActionWait        ComputerActionType = "wait"
)

// ComputerAction is implemented by every typed action struct. Use a
// type switch to determine the concrete type after parsing.
type ComputerAction interface {
	ActionType() ComputerActionType
}

// ClickAction represents a mouse click at the given coordinates.
type ClickAction struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Button string `json:"button"`
}

// ActionType implements ComputerAction.
func (a ClickAction) ActionType() ComputerActionType { return ComputerActionClick }

// DoubleClickAction represents a double-click at the given
// coordinates.
type DoubleClickAction struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// ActionType implements ComputerAction.
func (a DoubleClickAction) ActionType() ComputerActionType { return ComputerActionDoubleClick }

// DragPoint is a single waypoint along a drag path.
type DragPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// DragAction represents a mouse drag from a starting position along
// a series of waypoints.
type DragAction struct {
	StartX int         `json:"start_x"`
	StartY int         `json:"start_y"`
	Path   []DragPoint `json:"path"`
}

// ActionType implements ComputerAction.
func (a DragAction) ActionType() ComputerActionType { return ComputerActionDrag }

// KeypressAction represents one or more key presses.
type KeypressAction struct {
	Keys []string `json:"keys"`
}

// ActionType implements ComputerAction.
func (a KeypressAction) ActionType() ComputerActionType { return ComputerActionKeypress }

// MoveAction represents moving the cursor to the given coordinates.
type MoveAction struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// ActionType implements ComputerAction.
func (a MoveAction) ActionType() ComputerActionType { return ComputerActionMove }

// ScreenshotAction requests a screenshot of the current display.
type ScreenshotAction struct{}

// ActionType implements ComputerAction.
func (a ScreenshotAction) ActionType() ComputerActionType { return ComputerActionScreenshot }

// ScrollAction represents a scroll event at the given coordinates.
type ScrollAction struct {
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Direction string `json:"direction"`
	Amount    int    `json:"amount"`
}

// ActionType implements ComputerAction.
func (a ScrollAction) ActionType() ComputerActionType { return ComputerActionScroll }

// TypeAction represents typing a string of text.
type TypeAction struct {
	Text string `json:"text"`
}

// ActionType implements ComputerAction.
func (a TypeAction) ActionType() ComputerActionType { return ComputerActionTyping }

// WaitAction requests a pause before the next action.
type WaitAction struct{}

// ActionType implements ComputerAction.
func (a WaitAction) ActionType() ComputerActionType { return ComputerActionWait }

// parseSingleAction unmarshals a single JSON object into the correct
// typed action struct based on the "type" discriminator field.
func parseSingleAction(data json.RawMessage) (ComputerAction, error) {
	var disc struct {
		Type ComputerActionType `json:"type"`
	}
	if err := json.Unmarshal(data, &disc); err != nil {
		return nil, fmt.Errorf("unmarshal action type: %w", err)
	}

	switch disc.Type {
	case ComputerActionClick:
		var a ClickAction
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("unmarshal click action: %w", err)
		}
		return a, nil
	case ComputerActionDoubleClick:
		var a DoubleClickAction
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("unmarshal double_click action: %w", err)
		}
		return a, nil
	case ComputerActionDrag:
		var a DragAction
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("unmarshal drag action: %w", err)
		}
		return a, nil
	case ComputerActionKeypress:
		var a KeypressAction
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("unmarshal keypress action: %w", err)
		}
		return a, nil
	case ComputerActionMove:
		var a MoveAction
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("unmarshal move action: %w", err)
		}
		return a, nil
	case ComputerActionScreenshot:
		var a ScreenshotAction
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("unmarshal screenshot action: %w", err)
		}
		return a, nil
	case ComputerActionScroll:
		var a ScrollAction
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("unmarshal scroll action: %w", err)
		}
		return a, nil
	case ComputerActionTyping:
		var a TypeAction
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("unmarshal type action: %w", err)
		}
		return a, nil
	case ComputerActionWait:
		var a WaitAction
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("unmarshal wait action: %w", err)
		}
		return a, nil
	default:
		return nil, fmt.Errorf("unknown computer action type: %q", disc.Type)
	}
}

// ParseComputerActions parses the input string from a computer use
// tool call into a slice of typed actions. The input may be a JSON
// array (batched actions) or a single JSON object.
func ParseComputerActions(input string) ([]ComputerAction, error) {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty computer action input")
	}

	// Determine whether the input is an array or a single object.
	if trimmed[0] == '[' {
		var rawItems []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &rawItems); err != nil {
			return nil, fmt.Errorf("unmarshal action array: %w", err)
		}
		actions := make([]ComputerAction, 0, len(rawItems))
		for i, raw := range rawItems {
			a, err := parseSingleAction(raw)
			if err != nil {
				return nil, fmt.Errorf("action[%d]: %w", i, err)
			}
			actions = append(actions, a)
		}
		return actions, nil
	}

	a, err := parseSingleAction(json.RawMessage(trimmed))
	if err != nil {
		return nil, err
	}
	return []ComputerAction{a}, nil
}

// ComputerHandler handles computer use actions requested by the
// model. Implement the methods for actions you want to support.
// Return a screenshot (base64 PNG data) after executing each
// action.
type ComputerHandler interface {
	Click(ctx context.Context, action ClickAction) (screenshot []byte, err error)
	DoubleClick(ctx context.Context, action DoubleClickAction) (screenshot []byte, err error)
	Drag(ctx context.Context, action DragAction) (screenshot []byte, err error)
	Keypress(ctx context.Context, action KeypressAction) (screenshot []byte, err error)
	Move(ctx context.Context, action MoveAction) (screenshot []byte, err error)
	Screenshot(ctx context.Context, action ScreenshotAction) (screenshot []byte, err error)
	Scroll(ctx context.Context, action ScrollAction) (screenshot []byte, err error)
	Type(ctx context.Context, action TypeAction) (screenshot []byte, err error)
	Wait(ctx context.Context, action WaitAction) (screenshot []byte, err error)
}

// ExecuteComputerActions parses the actions from a tool call input
// and dispatches each one to the appropriate handler method. It
// returns the screenshot from the last action executed, which is
// what gets sent back to the model.
func ExecuteComputerActions(ctx context.Context, handler ComputerHandler, input string) (screenshot []byte, err error) {
	actions, err := ParseComputerActions(input)
	if err != nil {
		return nil, fmt.Errorf("parse computer actions: %w", err)
	}
	if len(actions) == 0 {
		return nil, fmt.Errorf("no computer actions to execute")
	}

	for _, action := range actions {
		switch a := action.(type) {
		case ClickAction:
			screenshot, err = handler.Click(ctx, a)
		case DoubleClickAction:
			screenshot, err = handler.DoubleClick(ctx, a)
		case DragAction:
			screenshot, err = handler.Drag(ctx, a)
		case KeypressAction:
			screenshot, err = handler.Keypress(ctx, a)
		case MoveAction:
			screenshot, err = handler.Move(ctx, a)
		case ScreenshotAction:
			screenshot, err = handler.Screenshot(ctx, a)
		case ScrollAction:
			screenshot, err = handler.Scroll(ctx, a)
		case TypeAction:
			screenshot, err = handler.Type(ctx, a)
		case WaitAction:
			screenshot, err = handler.Wait(ctx, a)
		default:
			return nil, fmt.Errorf(
				"unhandled computer action type: %T", action,
			)
		}
		if err != nil {
			return nil, fmt.Errorf(
				"execute %s action: %w", action.ActionType(), err,
			)
		}
	}

	return screenshot, nil
}
