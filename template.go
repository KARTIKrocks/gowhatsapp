package whatsapp

import "strconv"

// TemplateMessage is a pre-approved message template send. Templates are the
// only way to start a conversation or message a user outside the open 24-hour
// service window. Create and submit templates for approval in WhatsApp Manager
// (template management via API is a future addition); reference them here by
// Name plus Language.
type TemplateMessage struct {
	// Name is the approved template name.
	Name string
	// Language is the BCP-47 / Meta locale code (e.g. "en", "en_US"). Defaults
	// to "en_US" when empty.
	Language string
	// Components fill the template's header/body/button placeholders, in order.
	Components []TemplateComponent
}

func (t TemplateMessage) messageType() string { return "template" }

func (t TemplateMessage) buildRequest(to string) map[string]any {
	lang := t.Language
	if lang == "" {
		lang = "en_US"
	}
	tmpl := map[string]any{
		"name":     t.Name,
		"language": map[string]any{"code": lang},
	}
	if len(t.Components) > 0 {
		comps := make([]map[string]any, 0, len(t.Components))
		for _, c := range t.Components {
			comps = append(comps, c.build())
		}
		tmpl["components"] = comps
	}
	return envelope(to, "template", tmpl)
}

// TemplateComponent is one header, body, or button section of a template.
type TemplateComponent struct {
	// Type is "header", "body", or "button".
	Type string
	// SubType applies to buttons: "url", "quick_reply", "copy_code", "flow".
	SubType string
	// Index is the zero-based button position; required for button components.
	Index *int
	// Parameters fill the {{n}} placeholders within the component.
	Parameters []TemplateParameter
}

func (c TemplateComponent) build() map[string]any {
	m := map[string]any{"type": c.Type}
	if c.SubType != "" {
		m["sub_type"] = c.SubType
	}
	if c.Index != nil {
		// The Cloud API expects the button index as a string.
		m["index"] = strconv.Itoa(*c.Index)
	}
	if len(c.Parameters) > 0 {
		ps := make([]map[string]any, 0, len(c.Parameters))
		for _, p := range c.Parameters {
			ps = append(ps, p.build())
		}
		m["parameters"] = ps
	}
	return m
}

// TemplateParameter is a single substitution value for a template placeholder.
// For the common text case use TextParam; richer parameter kinds (currency,
// date_time, media) are supported by setting Type accordingly.
type TemplateParameter struct {
	// Type is "text", "payload", "coupon_code", "currency", "date_time",
	// "image", "document", or "video".
	Type string
	// Text carries the value for text/payload/coupon_code parameters.
	Text string
}

func (p TemplateParameter) build() map[string]any {
	m := map[string]any{"type": p.Type}
	switch p.Type {
	case "payload":
		m["payload"] = p.Text
	case "coupon_code":
		m["coupon_code"] = p.Text
	default:
		m["text"] = p.Text
	}
	return m
}

// TextParam builds a text template parameter.
func TextParam(text string) TemplateParameter {
	return TemplateParameter{Type: "text", Text: text}
}

// BodyComponent builds a "body" component from positional text parameters.
func BodyComponent(params ...string) TemplateComponent {
	ps := make([]TemplateParameter, 0, len(params))
	for _, p := range params {
		ps = append(ps, TextParam(p))
	}
	return TemplateComponent{Type: "body", Parameters: ps}
}

// AuthTemplate builds an authentication-category template carrying a one-time
// code, the standard path for OTP delivery over WhatsApp. It fills the body
// placeholder with the code and the copy-code / one-tap URL button parameter
// (button index 0) that Meta's authentication templates require.
//
// Important: Meta does NOT generate or verify the code — it only delivers it.
// Generation and verification stay in your application. This helper just
// formats the delivery payload.
func AuthTemplate(name, language, code string) TemplateMessage {
	zero := 0
	return TemplateMessage{
		Name:     name,
		Language: language,
		Components: []TemplateComponent{
			{Type: "body", Parameters: []TemplateParameter{TextParam(code)}},
			{Type: "button", SubType: "url", Index: &zero, Parameters: []TemplateParameter{TextParam(code)}},
		},
	}
}
