package teams

import (
	"encoding/json"
	"errors"
)

// ErrNotInvoke means the activity does not carry a recognisable quill
// invoke payload — the caller should fall through to the regular
// message-handling path.
var ErrNotInvoke = errors.New("teams: activity is not a quill invoke")

// InvokeData is the decoded Action.Submit payload our cards send back.
// All optional fields use omitempty so a single struct works for both
// switch_mode and switch_model.
type InvokeData struct {
	Action string `json:"quill.action"`
	Thread string `json:"thread"`
	Mode   string `json:"mode,omitempty"`
	Model  string `json:"model,omitempty"`
}

// UnmarshalInvokeData decodes activity.Value into an InvokeData. Returns
// ErrNotInvoke (wrapped or sentinel) when the payload is missing or
// lacks a quill.action key, so the dispatcher knows to fall through.
func UnmarshalInvokeData(activity *Activity) (InvokeData, error) {
	if len(activity.Value) == 0 {
		return InvokeData{}, ErrNotInvoke
	}
	// Reject scalar values (e.g. plain string) up front — json.Unmarshal
	// into a struct from a non-object would silently produce a zero value.
	if activity.Value[0] != '{' {
		return InvokeData{}, errors.New("teams: invoke value is not a JSON object")
	}
	var d InvokeData
	if err := json.Unmarshal(activity.Value, &d); err != nil {
		return InvokeData{}, err
	}
	if d.Action == "" {
		return InvokeData{}, ErrNotInvoke
	}
	return d, nil
}
