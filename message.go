package whatsapp

import "fmt"

// validator is optionally implemented by Message types that can reject
// themselves before a request is built — e.g. a Media referencing neither an
// uploaded ID nor a public Link. Send and SendReply call validate when present.
type validator interface {
	validate() error
}

// Message is a sendable WhatsApp message. The interface is sealed: only this
// package can implement it (via the unexported buildRequest method). That is a
// deliberate API-compatibility choice — new message types can be added in
// future releases without breaking callers, because no external code can hold
// an alternative implementation that the client wouldn't understand.
//
// Construct values directly (Text{...}) or via the typed constructors
// (TextMessage, ImageByLink, AuthTemplate, ...).
type Message interface {
	// buildRequest renders the full Cloud API request body addressed to `to`.
	buildRequest(to string) map[string]any

	// messageType returns the Cloud API "type" discriminator, for logging.
	messageType() string
}

func envelope(to, typ string, payload map[string]any) map[string]any {
	m := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              typ,
	}
	m[typ] = payload
	return m
}

// Text is a plain text message. Free-form text may only be sent inside an open
// 24-hour customer service window; outside it, use a Template.
type Text struct {
	Body string
	// PreviewURL enables link previews when Body contains a URL.
	PreviewURL bool
}

// TextMessage is a convenience constructor for a Text message.
func TextMessage(body string) Text { return Text{Body: body} }

func (t Text) messageType() string { return "text" }

func (t Text) buildRequest(to string) map[string]any {
	return envelope(to, "text", map[string]any{
		"body":        t.Body,
		"preview_url": t.PreviewURL,
	})
}

// Reaction reacts to a previously received message with an emoji. Send an
// empty Emoji to remove a reaction.
type Reaction struct {
	MessageID string
	Emoji     string
}

func (r Reaction) messageType() string { return "reaction" }

func (r Reaction) buildRequest(to string) map[string]any {
	return envelope(to, "reaction", map[string]any{
		"message_id": r.MessageID,
		"emoji":      r.Emoji,
	})
}

// Location is a location pin message.
type Location struct {
	Latitude  float64
	Longitude float64
	Name      string
	Address   string
}

func (l Location) messageType() string { return "location" }

func (l Location) buildRequest(to string) map[string]any {
	loc := map[string]any{
		"latitude":  l.Latitude,
		"longitude": l.Longitude,
	}
	if l.Name != "" {
		loc["name"] = l.Name
	}
	if l.Address != "" {
		loc["address"] = l.Address
	}
	return envelope(to, "location", loc)
}

// Media is an image, document, audio, video, or sticker message. Reference the
// asset either by a public Link or by an uploaded media ID (the Media upload
// API is a future addition); set exactly one. Caption applies to image, video,
// and document; Filename applies to document.
type Media struct {
	kind     string
	ID       string
	Link     string
	Caption  string
	Filename string
}

func (m Media) messageType() string { return m.kind }

// validate enforces that exactly one of ID (an uploaded media ID) or Link (a
// public URL) is set, giving a clear local error instead of an opaque API
// rejection.
func (m Media) validate() error {
	if (m.ID == "") == (m.Link == "") {
		return fmt.Errorf("%w: %s needs exactly one of ID or Link", ErrInvalidMessage, m.kind)
	}
	return nil
}

func (m Media) buildRequest(to string) map[string]any {
	obj := map[string]any{}
	if m.ID != "" {
		obj["id"] = m.ID
	} else {
		obj["link"] = m.Link
	}
	switch m.kind {
	case "image", "video", "document":
		if m.Caption != "" {
			obj["caption"] = m.Caption
		}
	}
	if m.kind == "document" && m.Filename != "" {
		obj["filename"] = m.Filename
	}
	return envelope(to, m.kind, obj)
}

// ImageByLink sends an image referenced by a public URL.
func ImageByLink(link, caption string) Media {
	return Media{kind: "image", Link: link, Caption: caption}
}

// ImageByID sends a previously uploaded image by media ID.
func ImageByID(id, caption string) Media {
	return Media{kind: "image", ID: id, Caption: caption}
}

// DocumentByLink sends a document referenced by a public URL.
func DocumentByLink(link, filename, caption string) Media {
	return Media{kind: "document", Link: link, Filename: filename, Caption: caption}
}

// DocumentByID sends a previously uploaded document by media ID.
func DocumentByID(id, filename, caption string) Media {
	return Media{kind: "document", ID: id, Filename: filename, Caption: caption}
}

// VideoByLink sends a video referenced by a public URL.
func VideoByLink(link, caption string) Media {
	return Media{kind: "video", Link: link, Caption: caption}
}

// VideoByID sends a previously uploaded video by media ID.
func VideoByID(id, caption string) Media {
	return Media{kind: "video", ID: id, Caption: caption}
}

// AudioByLink sends an audio clip referenced by a public URL.
func AudioByLink(link string) Media {
	return Media{kind: "audio", Link: link}
}

// AudioByID sends a previously uploaded audio clip by media ID.
func AudioByID(id string) Media {
	return Media{kind: "audio", ID: id}
}

// StickerByLink sends a sticker referenced by a public URL.
func StickerByLink(link string) Media {
	return Media{kind: "sticker", Link: link}
}

// StickerByID sends a previously uploaded sticker by media ID.
func StickerByID(id string) Media {
	return Media{kind: "sticker", ID: id}
}
