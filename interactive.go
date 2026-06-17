package whatsapp

// This file adds interactive message types — reply buttons and list menus —
// the building blocks of menu-driven WhatsApp bots. They are sent inside an open
// 24-hour service window (like free-form text). When a user taps a button or
// picks a list row, the choice arrives via the webhook as an inbound message
// whose Interactive field carries the selected ID and title (see
// [InboundMessage.Interactive]).

// Button is one quick-reply button in an [InteractiveButtons] message. ID is
// echoed back in the webhook when tapped; Title is the visible label.
type Button struct {
	ID    string
	Title string
}

// InteractiveButtons is a message with up to three quick-reply buttons. Body is
// required; Header (text) and Footer are optional.
type InteractiveButtons struct {
	Body    string
	Header  string
	Footer  string
	Buttons []Button
}

func (m InteractiveButtons) messageType() string { return "interactive" }

func (m InteractiveButtons) buildRequest(to string) map[string]any {
	buttons := make([]map[string]any, 0, len(m.Buttons))
	for _, b := range m.Buttons {
		buttons = append(buttons, map[string]any{
			"type":  "reply",
			"reply": map[string]any{"id": b.ID, "title": b.Title},
		})
	}
	interactive := map[string]any{
		"type":   "button",
		"body":   map[string]any{"text": m.Body},
		"action": map[string]any{"buttons": buttons},
	}
	if m.Header != "" {
		interactive["header"] = map[string]any{"type": "text", "text": m.Header}
	}
	if m.Footer != "" {
		interactive["footer"] = map[string]any{"text": m.Footer}
	}
	return envelope(to, "interactive", interactive)
}

// ListRow is one selectable row inside a [ListSection]. ID is echoed back in the
// webhook when chosen; Description is optional secondary text.
type ListRow struct {
	ID          string
	Title       string
	Description string
}

// ListSection groups rows under an optional heading in an [InteractiveList].
type ListSection struct {
	Title string
	Rows  []ListRow
}

// InteractiveList is a menu the user opens via a single button to pick one row
// from one or more sections. Body and ButtonText are required; Header (text)
// and Footer are optional.
type InteractiveList struct {
	Body       string
	Header     string
	Footer     string
	ButtonText string
	Sections   []ListSection
}

func (m InteractiveList) messageType() string { return "interactive" }

func (m InteractiveList) buildRequest(to string) map[string]any {
	sections := make([]map[string]any, 0, len(m.Sections))
	for _, s := range m.Sections {
		rows := make([]map[string]any, 0, len(s.Rows))
		for _, r := range s.Rows {
			row := map[string]any{"id": r.ID, "title": r.Title}
			if r.Description != "" {
				row["description"] = r.Description
			}
			rows = append(rows, row)
		}
		sec := map[string]any{"rows": rows}
		if s.Title != "" {
			sec["title"] = s.Title
		}
		sections = append(sections, sec)
	}
	interactive := map[string]any{
		"type": "list",
		"body": map[string]any{"text": m.Body},
		"action": map[string]any{
			"button":   m.ButtonText,
			"sections": sections,
		},
	}
	if m.Header != "" {
		interactive["header"] = map[string]any{"type": "text", "text": m.Header}
	}
	if m.Footer != "" {
		interactive["footer"] = map[string]any{"text": m.Footer}
	}
	return envelope(to, "interactive", interactive)
}
