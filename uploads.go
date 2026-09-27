package transcdr

import (
	"context"
	"io"
	"iter"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// UploadsService uploads source files directly to storage.
type UploadsService struct{ client *Client }

// Create opens an upload session. An Idempotency-Key is generated
// automatically, so a retry never opens a second session.
func (s *UploadsService) Create(ctx context.Context, params *UploadCreateParams, opts ...RequestOption) (*Upload, error) {
	var u Upload
	if err := s.client.do(ctx, "POST", "/v1/uploads", nil, params, &u, s.client.withAutoIdempotency(opts)); err != nil {
		return nil, err
	}
	return &u, nil
}

// Complete marks the upload finished and returns the ready asset.
func (s *UploadsService) Complete(ctx context.Context, id string, opts ...RequestOption) (*Asset, error) {
	return doJSON[Asset](ctx, s.client, "POST", "/v1/uploads/"+seg(id)+"/complete", nil, opts)
}

// UploadProgress reports bytes sent.
type UploadProgress struct {
	Loaded  int64
	Total   int64
	Percent float64
}

// UploadFileOptions tune [UploadsService.UploadFile].
type UploadFileOptions struct {
	// Filename defaults to the file's base name (UploadPath), else "upload".
	Filename string
	// ContentType defaults to the type of the file name's extension, else
	// application/octet-stream.
	ContentType string
	Metadata    Metadata
	// OnProgress is called as bytes are sent, from 0 to 100 %.
	OnProgress func(UploadProgress)
	// TransformUploadURL rewrites the upload URL before the PUT (e.g. to route
	// through a proxy).
	TransformUploadURL func(string) string
}

// UploadFile uploads size bytes from r in one call: it opens the session, PUTs
// the bytes to the upload URL (streaming, never buffering the whole file) and
// completes it. It returns the ready asset.
func (s *UploadsService) UploadFile(ctx context.Context, r io.Reader, size int64, opts *UploadFileOptions) (*Asset, error) {
	o := UploadFileOptions{}
	if opts != nil {
		o = *opts
	}
	if o.Filename == "" {
		o.Filename = "upload"
	}
	if o.ContentType == "" {
		o.ContentType = contentTypeFor(o.Filename)
		if o.ContentType == "" {
			o.ContentType = "application/octet-stream"
		}
	}

	upload, err := s.Create(ctx, &UploadCreateParams{Filename: o.Filename, ContentType: o.ContentType, SizeBytes: size, Metadata: o.Metadata})
	if err != nil {
		return nil, err
	}
	target := s.client.url(upload.UploadURL)
	if o.TransformUploadURL != nil {
		target = o.TransformUploadURL(target)
	}
	method := upload.UploadMethod
	if method == "" {
		method = http.MethodPut
	}

	report := func(n int64) {
		if o.OnProgress == nil {
			return
		}
		pct := 100.0
		if size > 0 {
			pct = min(100, float64(n)*100/float64(size))
		}
		o.OnProgress(UploadProgress{Loaded: n, Total: size, Percent: pct})
	}
	report(0)
	body := &progressReader{r: r, report: report}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	req.ContentLength = size
	if size == 0 {
		req.Body = http.NoBody
	}
	req.Header.Set("Content-Type", o.ContentType)
	for k, v := range upload.UploadHeaders {
		req.Header.Set(k, v)
	}
	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &ConnectionError{Message: "Upload failed: " + err.Error(), Err: err}
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errorFromResponse(resp.StatusCode, data, resp.Header)
	}
	report(size)
	return s.Complete(ctx, upload.ID)
}

// UploadPath uploads a local file: see [UploadsService.UploadFile]. The
// filename defaults to the file's base name.
func (s *UploadsService) UploadPath(ctx context.Context, path string, opts *UploadFileOptions) (*Asset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	o := UploadFileOptions{}
	if opts != nil {
		o = *opts
	}
	if o.Filename == "" {
		o.Filename = filepath.Base(path)
	}
	return s.UploadFile(ctx, f, info.Size(), &o)
}

// videoTypes are the media types of common video files: the system's MIME
// table (mime.TypeByExtension) often lacks them.
var videoTypes = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/x-m4v", ".mov": "video/quicktime", ".mkv": "video/x-matroska",
	".webm": "video/webm", ".avi": "video/x-msvideo", ".ts": "video/mp2t", ".mts": "video/mp2t",
	".m2ts": "video/mp2t", ".mxf": "application/mxf", ".mpg": "video/mpeg", ".mpeg": "video/mpeg",
	".wmv": "video/x-ms-wmv", ".flv": "video/x-flv", ".3gp": "video/3gpp",
}

// contentTypeFor is the media type of a file name's extension, or "".
func contentTypeFor(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if t, ok := videoTypes[ext]; ok {
		return t
	}
	return mime.TypeByExtension(ext)
}

type progressReader struct {
	r      io.Reader
	n      int64
	mu     sync.Mutex
	report func(int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.mu.Lock()
		p.n += int64(n)
		total := p.n
		p.mu.Unlock()
		p.report(total)
	}
	return n, err
}

// AssetsService manages source files.
type AssetsService struct{ client *Client }

// List returns one page of assets.
func (s *AssetsService) List(ctx context.Context, params *ListParams, opts ...RequestOption) (*Page[Asset], error) {
	return getPage[Asset](ctx, s.client, "/v1/assets", params.values(nil), opts)
}

// All iterates over every asset.
func (s *AssetsService) All(ctx context.Context, params *ListParams, opts ...RequestOption) iter.Seq2[Asset, error] {
	return iterate[Asset](ctx, s.client, "/v1/assets", params.values(nil), opts)
}

// Create links a remote file by URL. The asset is ready at once; jobs read
// the URL directly.
func (s *AssetsService) Create(ctx context.Context, params *AssetImportParams, opts ...RequestOption) (*Asset, error) {
	return doJSONBody[Asset](ctx, s.client, "POST", "/v1/assets", params, opts)
}

// Get retrieves an asset.
func (s *AssetsService) Get(ctx context.Context, id string, opts ...RequestOption) (*Asset, error) {
	return doJSON[Asset](ctx, s.client, "GET", "/v1/assets/"+seg(id), nil, opts)
}

// ContentURL returns a short-lived signed download URL for the original file.
func (s *AssetsService) ContentURL(ctx context.Context, id string, opts ...RequestOption) (*SignedURL, error) {
	return doJSON[SignedURL](ctx, s.client, "GET", "/v1/assets/"+seg(id)+"/content", url.Values{"redirect": {"false"}}, opts)
}

// Delete deletes an asset.
func (s *AssetsService) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return s.client.do(ctx, "DELETE", "/v1/assets/"+seg(id), nil, nil, nil, opts)
}
