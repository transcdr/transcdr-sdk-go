package transcdr

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func uploadSession(url string) string {
	return `{"object":"upload","id":"upl_1","asset_id":"ast_1","status":"pending","upload_url":"` + url + `",
		"upload_method":"PUT","upload_headers":{"x-amz-meta-origin":"sdk-test"},"expires_at":"2026-09-27T12:00:00Z"}`
}

const readyAsset = `{"object":"asset","id":"ast_1","status":"ready","filename":"clip.mp4","content_type":"video/mp4","size_bytes":11,"metadata":{},"download_url":"u","created_at":"2026-09-27T10:00:00Z"}`

func TestUploadsCreateAndComplete(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(201, uploadSession("https://storage.example.com/put"))
	u, err := f.client().Uploads.Create(context.Background(), &UploadCreateParams{Filename: "a.mov", ContentType: "video/quicktime", SizeBytes: 42, Metadata: Metadata{"k": "v"}})
	must(t, err)
	r := f.expect("POST", "/v1/uploads")
	if !uuidShape(r.Header.Get("Idempotency-Key")) {
		t.Fatalf("Idempotency-Key = %q", r.Header.Get("Idempotency-Key"))
	}
	if string(r.Body) != `{"filename":"a.mov","content_type":"video/quicktime","size_bytes":42,"metadata":{"k":"v"}}` {
		t.Fatalf("body = %s", r.Body)
	}
	if u.ID != "upl_1" || u.UploadHeaders["x-amz-meta-origin"] != "sdk-test" || u.ExpiresAt.IsZero() {
		t.Fatalf("upload = %+v", u)
	}

	f.reply(200, readyAsset)
	a, err := f.client().Uploads.Complete(context.Background(), "upl_1")
	must(t, err)
	f.expect("POST", "/v1/uploads/upl_1/complete")
	if a.Status != "ready" {
		t.Fatalf("asset = %+v", a)
	}
}

func TestUploadFile(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(201, uploadSession(f.server.URL+"/upload/xyz")).reply(200, "").reply(200, readyAsset)
	content := []byte("fake video!")
	var progress []UploadProgress
	asset, err := f.client().Uploads.UploadFile(context.Background(), bytes.NewReader(content), int64(len(content)), &UploadFileOptions{
		Filename:   "clip.mp4",
		Metadata:   Metadata{"source": "test"},
		OnProgress: func(p UploadProgress) { progress = append(progress, p) },
	})
	must(t, err)
	if asset.ID != "ast_1" {
		t.Fatalf("asset = %+v", asset)
	}
	reqs := f.all()
	if len(reqs) != 3 {
		t.Fatalf("requests = %d", len(reqs))
	}
	create := reqs[0].JSON(t)
	if create["filename"] != "clip.mp4" || create["content_type"] != "video/mp4" || create["size_bytes"] != float64(len(content)) {
		t.Fatalf("session body = %s", reqs[0].Body)
	}
	put := reqs[1]
	if put.Method != "PUT" || put.Path != "/upload/xyz" || !bytes.Equal(put.Body, content) {
		t.Fatalf("PUT %s %s %q", put.Method, put.Path, put.Body)
	}
	if put.Header.Get("Content-Type") != "video/mp4" || put.Header.Get("X-Amz-Meta-Origin") != "sdk-test" {
		t.Fatalf("PUT headers = %v", put.Header)
	}
	if put.Header.Get("Authorization") != "" {
		t.Fatal("the upload URL must not get the API key")
	}
	if reqs[2].Method != "POST" || reqs[2].Path != "/v1/uploads/upl_1/complete" {
		t.Fatalf("complete = %s %s", reqs[2].Method, reqs[2].Path)
	}
	checkProgress(t, progress, int64(len(content)))
}

func checkProgress(t *testing.T, progress []UploadProgress, size int64) {
	t.Helper()
	if len(progress) < 2 {
		t.Fatalf("progress = %+v", progress)
	}
	first, last := progress[0], progress[len(progress)-1]
	if first.Loaded != 0 || first.Percent != 0 || first.Total != size {
		t.Fatalf("first progress = %+v", first)
	}
	if last.Loaded != size || last.Percent != 100 || last.Total != size {
		t.Fatalf("last progress = %+v", last)
	}
	for i := 1; i < len(progress); i++ {
		if progress[i].Loaded < progress[i-1].Loaded {
			t.Fatalf("progress went backwards: %+v", progress)
		}
	}
}

func TestUploadFileStreamsANonSeekableReader(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(201, uploadSession("/upload/relative")).reply(200, "").reply(200, readyAsset)
	parts := []string{strings.Repeat("a", 40000), strings.Repeat("b", 40000), "end"}
	r := io.MultiReader(strings.NewReader(parts[0]), strings.NewReader(parts[1]), strings.NewReader(parts[2]))
	size := int64(len(parts[0]) + len(parts[1]) + len(parts[2]))
	var progress []UploadProgress
	_, err := f.client().Uploads.UploadFile(context.Background(), r, size, &UploadFileOptions{OnProgress: func(p UploadProgress) { progress = append(progress, p) }})
	must(t, err)
	reqs := f.all()
	// A relative upload URL resolves against the API.
	if reqs[1].Path != "/upload/relative" || string(reqs[1].Body) != strings.Join(parts, "") {
		t.Fatalf("PUT %s, %d bytes", reqs[1].Path, len(reqs[1].Body))
	}
	// Defaults: filename "upload", octet-stream.
	if b := reqs[0].JSON(t); b["filename"] != "upload" || b["content_type"] != "application/octet-stream" {
		t.Fatalf("session body = %s", reqs[0].Body)
	}
	checkProgress(t, progress, size)
}

func TestUploadPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keynote.mov")
	content := []byte("quicktime bytes")
	must(t, os.WriteFile(path, content, 0o600))

	f := newFakeAPI(t)
	f.reply(201, uploadSession(f.server.URL+"/upload/1")).reply(200, "").reply(200, readyAsset)
	_, err := f.client().Uploads.UploadPath(context.Background(), path, nil)
	must(t, err)
	reqs := f.all()
	b := reqs[0].JSON(t)
	if b["filename"] != "keynote.mov" || b["content_type"] != "video/quicktime" || b["size_bytes"] != float64(len(content)) {
		t.Fatalf("session body = %s", reqs[0].Body)
	}
	if !bytes.Equal(reqs[1].Body, content) || reqs[1].Header.Get("Content-Type") != "video/quicktime" {
		t.Fatalf("PUT body %q, type %q", reqs[1].Body, reqs[1].Header.Get("Content-Type"))
	}

	if _, err := f.client().Uploads.UploadPath(context.Background(), filepath.Join(dir, "missing.mp4"), nil); !os.IsNotExist(err) {
		t.Fatalf("missing file: %v", err)
	}
}

func TestUploadStorageError(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(201, uploadSession(f.server.URL+"/upload/denied")).
		reply(403, `<?xml version="1.0"?><Error><Code>AccessDenied</Code></Error>`, "X-Request-Id", "store-1")
	_, err := f.client().Uploads.UploadFile(context.Background(), strings.NewReader("x"), 1, nil)
	e, ok := AsError(err)
	if !ok || e.Status != 403 || !IsPermission(err) || !strings.Contains(e.Message, "AccessDenied") || e.RequestID != "store-1" {
		t.Fatalf("err = %#v", err)
	}
	if n := len(f.all()); n != 2 {
		t.Fatalf("the upload was completed after a failed PUT (%d requests)", n)
	}
}

func TestUploadTransformURL(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(201, uploadSession("https://storage.invalid/put?sig=1")).reply(200, "").reply(200, readyAsset)
	var seen string
	_, err := f.client().Uploads.UploadFile(context.Background(), strings.NewReader("abc"), 3, &UploadFileOptions{
		ContentType: "video/mp4",
		TransformUploadURL: func(u string) string {
			seen = u
			return f.server.URL + "/proxied"
		},
	})
	must(t, err)
	if seen != "https://storage.invalid/put?sig=1" || f.all()[1].Path != "/proxied" {
		t.Fatalf("seen %q, PUT %s", seen, f.all()[1].Path)
	}
}

func TestAssets(t *testing.T) {
	f := newFakeAPI(t)
	ctx := context.Background()
	c := f.client()

	f.reply(200, fixture(t, "assets.json"))
	page, err := c.Assets.List(ctx, &ListParams{Limit: 5})
	must(t, err)
	r := f.expect("GET", "/v1/assets")
	if qv(r.Query, "limit") != "5" || len(page.Data) != 5 || !page.HasMore {
		t.Fatalf("query %v, page %d %v", r.Query, len(page.Data), page.HasMore)
	}
	a := page.Data[0]
	if a.ID != "ast_mS8WEuP2joqZkCGyVc2DYZ" || a.Filename != "sample-camera.mp4" || a.Status != "ready" || a.SizeBytes != 1853328 || a.CreatedAt.IsZero() {
		t.Fatalf("asset = %+v", a)
	}

	f.reply(200, `{"data":[{"id":"ast_1"}],"has_more":false}`)
	all, err := Collect(c.Assets.All(ctx, nil), 0)
	must(t, err)
	if len(all) != 1 {
		t.Fatalf("all = %v", all)
	}

	f.reply(201, readyAsset)
	_, err = c.Assets.Create(ctx, &AssetImportParams{URL: "https://example.com/a.mp4", Filename: "a.mp4"})
	must(t, err)
	r = f.expect("POST", "/v1/assets")
	if string(r.Body) != `{"url":"https://example.com/a.mp4","filename":"a.mp4"}` {
		t.Fatalf("body = %s", r.Body)
	}

	f.reply(200, readyAsset)
	_, err = c.Assets.Get(ctx, "ast_1")
	must(t, err)
	f.expect("GET", "/v1/assets/ast_1")

	f.reply(200, `{"url":"https://example.com/dl","expires_at":"2026-09-27T12:00:00Z"}`)
	u, err := c.Assets.ContentURL(ctx, "ast_1")
	must(t, err)
	r = f.expect("GET", "/v1/assets/ast_1/content")
	if qv(r.Query, "redirect") != "false" || u.URL != "https://example.com/dl" {
		t.Fatalf("query %v, url %+v", r.Query, u)
	}

	f.reply(204, "")
	must(t, c.Assets.Delete(ctx, "ast_1"))
	f.expect("DELETE", "/v1/assets/ast_1")
}

func TestUploadSendsContentLength(t *testing.T) {
	var putLength int64 = -2
	var putEncoding []string
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("POST /v1/uploads", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, uploadSession(srv.URL+"/put"))
	})
	mux.HandleFunc("PUT /put", func(w http.ResponseWriter, r *http.Request) {
		putLength, putEncoding = r.ContentLength, r.TransferEncoding
		_, _ = io.Copy(io.Discard, r.Body)
	})
	mux.HandleFunc("POST /v1/uploads/upl_1/complete", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, readyAsset)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	// A reader of unknown length (not a *bytes.Reader) is still sent with the
	// declared size, never chunked.
	body := io.MultiReader(strings.NewReader("12345"), strings.NewReader("678"))
	_, err := NewClient(WithBaseURL(srv.URL), WithAPIKey("tdk_test_example")).Uploads.UploadFile(context.Background(), body, 8, nil)
	must(t, err)
	if putLength != 8 || len(putEncoding) != 0 {
		t.Fatalf("Content-Length %d, Transfer-Encoding %v", putLength, putEncoding)
	}
}

func TestContentTypeForVideoExtensions(t *testing.T) {
	for name, want := range map[string]string{
		"a.mp4": "video/mp4", "b.MOV": "video/quicktime", "c.mkv": "video/x-matroska", "d.webm": "video/webm",
		"e.m4v": "video/x-m4v", "f.ts": "video/mp2t", "g.mxf": "application/mxf", "h.unknownext": "",
	} {
		if got := contentTypeFor(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}
