package teams

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalInvokeData_HappyPath_Mode(t *testing.T) {
	a := &Activity{
		Type:  "message",
		Value: json.RawMessage(`{"quill.action":"switch_mode","thread":"teams:a:abc","mode":"kiro_spec"}`),
	}

	data, err := UnmarshalInvokeData(a)
	if err != nil {
		t.Fatalf("UnmarshalInvokeData: %v", err)
	}
	if data.Action != "switch_mode" {
		t.Errorf("Action = %q, want %q", data.Action, "switch_mode")
	}
	if data.Thread != "teams:a:abc" {
		t.Errorf("Thread = %q, want %q", data.Thread, "teams:a:abc")
	}
	if data.Mode != "kiro_spec" {
		t.Errorf("Mode = %q, want %q", data.Mode, "kiro_spec")
	}
}

func TestUnmarshalInvokeData_HappyPath_Model(t *testing.T) {
	a := &Activity{
		Type:  "message",
		Value: json.RawMessage(`{"quill.action":"switch_model","thread":"teams:a:abc","model":"claude-opus-4.6"}`),
	}
	data, err := UnmarshalInvokeData(a)
	if err != nil {
		t.Fatalf("UnmarshalInvokeData: %v", err)
	}
	if data.Action != "switch_model" {
		t.Errorf("Action = %q", data.Action)
	}
	if data.Model != "claude-opus-4.6" {
		t.Errorf("Model = %q", data.Model)
	}
}

func TestUnmarshalInvokeData_NoValue_NotInvoke(t *testing.T) {
	a := &Activity{Type: "message", Text: "hi"}

	_, err := UnmarshalInvokeData(a)
	if err == nil {
		t.Fatal("expected error when Value is empty")
	}
	if !errIsNotInvoke(err) {
		t.Errorf("expected ErrNotInvoke, got %v", err)
	}
}

func TestUnmarshalInvokeData_MissingAction(t *testing.T) {
	a := &Activity{
		Type:  "message",
		Value: json.RawMessage(`{"thread":"teams:a:abc"}`),
	}
	_, err := UnmarshalInvokeData(a)
	if err == nil {
		t.Fatal("expected error when quill.action missing")
	}
	if !errIsNotInvoke(err) {
		t.Errorf("expected ErrNotInvoke, got %v", err)
	}
}

func TestUnmarshalInvokeData_NonJSONValue(t *testing.T) {
	a := &Activity{
		Type:  "message",
		Value: json.RawMessage(`"some string"`),
	}
	_, err := UnmarshalInvokeData(a)
	if err == nil {
		t.Fatal("expected error when Value is not a JSON object")
	}
}

// errIsNotInvoke is a tiny helper that uses errors.Is — defined in the
// production package, but since invoke.go declares the sentinel we just
// reach for it directly.
func errIsNotInvoke(err error) bool {
	return err != nil && (err == ErrNotInvoke || err.Error() == ErrNotInvoke.Error())
}
