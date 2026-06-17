package whatsapp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	whatsapp "github.com/KARTIKrocks/gowhatsapp"
)

func TestUpload_BuildsMultipartAndParsesID(t *testing.T) {
	mt := whatsapp.NewMockTransport().WithResponse(200, `{"id":"media-123"}`)
	c := newTestClient(t, mt)

	id, err := c.Upload(context.Background(), "pic.jpg", "image/jpeg", strings.NewReader("JPEGBYTES"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if id != "media-123" {
		t.Fatalf("id = %q, want media-123", id)
	}
	req, _ := mt.LastRequest()
	if !strings.HasSuffix(req.URL, "/PNID/media") {
		t.Fatalf("URL = %q, want .../PNID/media", req.URL)
	}
	if ct := req.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data") {
		t.Fatalf("Content-Type = %q, want multipart/form-data", ct)
	}
	raw := string(req.RawBody)
	for _, want := range []string{"messaging_product", "whatsapp", "image/jpeg", `filename="pic.jpg"`, "JPEGBYTES"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("multipart body missing %q:\n%s", want, raw)
		}
	}
}

func TestUpload_RejectsEmptyMime(t *testing.T) {
	c := newTestClient(t, whatsapp.NewMockTransport())
	if _, err := c.Upload(context.Background(), "f", "", strings.NewReader("x")); err == nil {
		t.Fatal("want error for empty MIME type")
	}
}

func TestMediaInfoAndDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/MID"):
			// Point the download URL back at this same server.
			_, _ = w.Write([]byte(`{"id":"MID","url":"` + serverURL(r) + `/dl","mime_type":"image/png","sha256":"abc","file_size":9}`))
		case strings.HasSuffix(r.URL.Path, "/dl"):
			if got := r.Header.Get("Authorization"); got != "Bearer TOKEN" {
				t.Errorf("download Authorization = %q, want Bearer TOKEN", got)
			}
			_, _ = w.Write([]byte("PNG-DATA-"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := whatsapp.New(whatsapp.Config{PhoneNumberID: "PNID", AccessToken: "TOKEN"},
		whatsapp.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	info, err := c.MediaInfo(context.Background(), "MID")
	if err != nil {
		t.Fatalf("MediaInfo: %v", err)
	}
	if info.MimeType != "image/png" || info.FileSize != 9 {
		t.Fatalf("info = %+v", info)
	}

	data, mime, err := c.Download(context.Background(), "MID")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if string(data) != "PNG-DATA-" || mime != "image/png" {
		t.Fatalf("download data=%q mime=%q", data, mime)
	}
}

func TestDeleteMedia(t *testing.T) {
	mt := whatsapp.NewMockTransport().WithResponse(200, `{"success":true}`)
	c := newTestClient(t, mt)

	if err := c.DeleteMedia(context.Background(), "MID"); err != nil {
		t.Fatalf("DeleteMedia: %v", err)
	}
	req, _ := mt.LastRequest()
	if req.Method != http.MethodDelete {
		t.Fatalf("method = %s, want DELETE", req.Method)
	}
	if !strings.HasSuffix(req.URL, "/MID") {
		t.Fatalf("URL = %q, want .../MID", req.URL)
	}
}

// serverURL reconstructs the http://host base for the test server from a request.
func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
