package processor

import (
	"encoding/json"
	"testing"
)

func Test_State_MarshalJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(struct {
		State State `json:"state"`
	}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != `{"state":null}` {
		t.Fatalf("unexpected result: %s", data)
	}

	data, err = json.Marshal(State(`{"a":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != `{"a":1}` {
		t.Fatalf("unexpected result: %s", data)
	}
}

func Test_Settings_MarshalJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(struct {
		Settings Settings `json:"settings"`
	}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != `{"settings":null}` {
		t.Fatalf("unexpected result: %s", data)
	}

	data, err = json.Marshal(Settings(`{"a":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != `{"a":1}` {
		t.Fatalf("unexpected result: %s", data)
	}
}
