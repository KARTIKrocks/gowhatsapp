package whatsapp_test

import (
	"context"
	"testing"

	whatsapp "github.com/KARTIKrocks/gowhatsapp"
)

func TestInteractiveButtons_BuildsRequest(t *testing.T) {
	mt := whatsapp.NewMockTransport()
	c := newTestClient(t, mt)

	msg := whatsapp.InteractiveButtons{
		Body:   "Pick one",
		Header: "Menu",
		Footer: "footer",
		Buttons: []whatsapp.Button{
			{ID: "yes", Title: "Yes"},
			{ID: "no", Title: "No"},
		},
	}
	if _, err := c.Send(context.Background(), "1555", msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	req, _ := mt.LastRequest()
	if req.Body["type"] != "interactive" {
		t.Fatalf("type = %v, want interactive", req.Body["type"])
	}
	in, _ := req.Body["interactive"].(map[string]any)
	if in["type"] != "button" {
		t.Fatalf("interactive.type = %v, want button", in["type"])
	}
	action, _ := in["action"].(map[string]any)
	buttons, _ := action["buttons"].([]any)
	if len(buttons) != 2 {
		t.Fatalf("buttons = %d, want 2", len(buttons))
	}
	b0, _ := buttons[0].(map[string]any)
	reply, _ := b0["reply"].(map[string]any)
	if b0["type"] != "reply" || reply["id"] != "yes" || reply["title"] != "Yes" {
		t.Fatalf("button[0] = %#v", b0)
	}
	if hdr, _ := in["header"].(map[string]any); hdr["text"] != "Menu" {
		t.Fatalf("header = %#v", in["header"])
	}
}

func TestInteractiveList_BuildsRequest(t *testing.T) {
	mt := whatsapp.NewMockTransport()
	c := newTestClient(t, mt)

	msg := whatsapp.InteractiveList{
		Body:       "Choose",
		ButtonText: "Open",
		Sections: []whatsapp.ListSection{{
			Title: "Fruit",
			Rows: []whatsapp.ListRow{
				{ID: "a", Title: "Apple", Description: "red"},
				{ID: "b", Title: "Banana"},
			},
		}},
	}
	if _, err := c.Send(context.Background(), "1555", msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	req, _ := mt.LastRequest()
	in, _ := req.Body["interactive"].(map[string]any)
	if in["type"] != "list" {
		t.Fatalf("interactive.type = %v, want list", in["type"])
	}
	action, _ := in["action"].(map[string]any)
	if action["button"] != "Open" {
		t.Fatalf("action.button = %v, want Open", action["button"])
	}
	sections, _ := action["sections"].([]any)
	if len(sections) != 1 {
		t.Fatalf("sections = %d, want 1", len(sections))
	}
	sec, _ := sections[0].(map[string]any)
	rows, _ := sec["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	r0, _ := rows[0].(map[string]any)
	if r0["id"] != "a" || r0["title"] != "Apple" || r0["description"] != "red" {
		t.Fatalf("row[0] = %#v", r0)
	}
	// Row without a description must omit the key entirely.
	r1, _ := rows[1].(map[string]any)
	if _, ok := r1["description"]; ok {
		t.Fatalf("row[1] should omit description: %#v", r1)
	}
}
