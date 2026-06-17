package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
)

// MediaInfo is the metadata WhatsApp stores for an uploaded or received media
// asset. URL is a short-lived, authenticated download link (fetch it via
// [Client.Download], which adds the required bearer token).
type MediaInfo struct {
	ID       string
	URL      string
	MimeType string
	SHA256   string
	FileSize int64
}

// Upload stores a media file with WhatsApp and returns its media ID, which can
// then be sent with [ImageByID] (and the other *ByID constructors). mimeType
// must be a content type WhatsApp accepts for the asset (e.g. "image/jpeg").
// The reader is consumed fully and buffered in memory.
func (c *Client) Upload(ctx context.Context, filename, mimeType string, r io.Reader) (string, error) {
	if mimeType == "" {
		return "", fmt.Errorf("%w: empty media MIME type", ErrInvalidMessage)
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("messaging_product", "whatsapp"); err != nil {
		return "", fmt.Errorf("whatsapp: build upload: %w", err)
	}
	if err := w.WriteField("type", mimeType); err != nil {
		return "", fmt.Errorf("whatsapp: build upload: %w", err)
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	h.Set("Content-Type", mimeType)
	part, err := w.CreatePart(h)
	if err != nil {
		return "", fmt.Errorf("whatsapp: build upload: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return "", fmt.Errorf("whatsapp: read media: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("whatsapp: build upload: %w", err)
	}

	body, err := c.tr.postRaw(ctx, c.phoneNumberID+"/media", buf.Bytes(), w.FormDataContentType())
	if err != nil {
		return "", err
	}
	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", fmt.Errorf("whatsapp: parse upload response: %w", err)
	}
	return res.ID, nil
}

// MediaInfo fetches metadata (including the short-lived download URL) for a
// media ID, typically one received on an [InboundMedia].
func (c *Client) MediaInfo(ctx context.Context, mediaID string) (*MediaInfo, error) {
	if mediaID == "" {
		return nil, fmt.Errorf("%w: empty media ID", ErrInvalidMessage)
	}
	body, err := c.tr.get(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	var res struct {
		ID       string `json:"id"`
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
		SHA256   string `json:"sha256"`
		FileSize int64  `json:"file_size"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("whatsapp: parse media info: %w", err)
	}
	return &MediaInfo{
		ID:       res.ID,
		URL:      res.URL,
		MimeType: res.MimeType,
		SHA256:   res.SHA256,
		FileSize: res.FileSize,
	}, nil
}

// Download fetches the bytes of a media asset by ID, returning the content and
// its MIME type. It resolves the asset's short-lived URL with [Client.MediaInfo]
// and then downloads it with the required authentication.
func (c *Client) Download(ctx context.Context, mediaID string) ([]byte, string, error) {
	info, err := c.MediaInfo(ctx, mediaID)
	if err != nil {
		return nil, "", err
	}
	data, err := c.tr.getURL(ctx, info.URL)
	if err != nil {
		return nil, "", err
	}
	return data, info.MimeType, nil
}

// DeleteMedia deletes an uploaded media asset by ID.
func (c *Client) DeleteMedia(ctx context.Context, mediaID string) error {
	if mediaID == "" {
		return fmt.Errorf("%w: empty media ID", ErrInvalidMessage)
	}
	_, err := c.tr.del(ctx, mediaID)
	return err
}
