package whatsapp_test

import (
	"context"
	"encoding/json"
	"testing"

	whatsapp "github.com/KARTIKrocks/gowhatsapp"
)

// sentPayload sends msg through a mock client and returns the decoded request body.
func sentPayload(t *testing.T, msg whatsapp.Message) map[string]any {
	t.Helper()
	mt := whatsapp.NewMockTransport()
	c := newTestClient(t, mt)
	if _, err := c.Send(context.Background(), "15551234567", msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	req, ok := mt.LastRequest()
	if !ok {
		t.Fatal("no request captured")
	}
	return req.Body
}

func TestText_Payload(t *testing.T) {
	body := sentPayload(t, whatsapp.Text{Body: "hi", PreviewURL: true})
	if body["messaging_product"] != "whatsapp" || body["type"] != "text" {
		t.Fatalf("envelope = %#v", body)
	}
	text := body["text"].(map[string]any)
	if text["body"] != "hi" || text["preview_url"] != true {
		t.Fatalf("text = %#v", text)
	}
}

func TestMedia_Payload(t *testing.T) {
	body := sentPayload(t, whatsapp.DocumentByLink("https://x/y.pdf", "y.pdf", "report"))
	if body["type"] != "document" {
		t.Fatalf("type = %v", body["type"])
	}
	doc := body["document"].(map[string]any)
	if doc["link"] != "https://x/y.pdf" || doc["filename"] != "y.pdf" || doc["caption"] != "report" {
		t.Fatalf("document = %#v", doc)
	}

	// Audio carries neither caption nor filename.
	audio := sentPayload(t, whatsapp.AudioByLink("https://x/a.mp3"))["audio"].(map[string]any)
	if _, ok := audio["caption"]; ok {
		t.Fatalf("audio should not have caption: %#v", audio)
	}
}

func TestLocationAndReaction_Payload(t *testing.T) {
	loc := sentPayload(t, whatsapp.Location{Latitude: 1.5, Longitude: 2.5, Name: "HQ"})["location"].(map[string]any)
	if loc["latitude"] != 1.5 || loc["name"] != "HQ" {
		t.Fatalf("location = %#v", loc)
	}
	if _, ok := loc["address"]; ok {
		t.Fatalf("empty address should be omitted: %#v", loc)
	}

	reaction := sentPayload(t, whatsapp.Reaction{MessageID: "wamid.X", Emoji: "👍"})["reaction"].(map[string]any)
	if reaction["message_id"] != "wamid.X" || reaction["emoji"] != "👍" {
		t.Fatalf("reaction = %#v", reaction)
	}
}

func TestAuthTemplate_Payload(t *testing.T) {
	body := sentPayload(t, whatsapp.AuthTemplate("otp_login", "en_US", "472913"))
	if body["type"] != "template" {
		t.Fatalf("type = %v", body["type"])
	}
	// Round-trip through JSON to assert the exact wire shape Meta expects.
	raw, _ := json.Marshal(body["template"])
	var tmpl struct {
		Name     string `json:"name"`
		Language struct {
			Code string `json:"code"`
		} `json:"language"`
		Components []struct {
			Type       string `json:"type"`
			SubType    string `json:"sub_type"`
			Index      string `json:"index"`
			Parameters []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"parameters"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &tmpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}
	if tmpl.Name != "otp_login" || tmpl.Language.Code != "en_US" {
		t.Fatalf("template head = %+v", tmpl)
	}
	if len(tmpl.Components) != 2 {
		t.Fatalf("components = %d, want 2 (body + button)", len(tmpl.Components))
	}
	body0 := tmpl.Components[0]
	if body0.Type != "body" || body0.Parameters[0].Text != "472913" {
		t.Fatalf("body component = %+v", body0)
	}
	btn := tmpl.Components[1]
	if btn.Type != "button" || btn.SubType != "url" || btn.Index != "0" || btn.Parameters[0].Text != "472913" {
		t.Fatalf("button component = %+v", btn)
	}
}

func TestBodyComponent_Builder(t *testing.T) {
	tmpl := whatsapp.TemplateMessage{
		Name:       "match_mutual",
		Components: []whatsapp.TemplateComponent{whatsapp.BodyComponent("Asha", "Bengaluru")},
	}
	body := sentPayload(t, tmpl)
	raw, _ := json.Marshal(body["template"])
	if got := string(raw); !contains(got, `"en_US"`) {
		t.Fatalf("default language not applied: %s", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
