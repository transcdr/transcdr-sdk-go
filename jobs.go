package transcdr

import (
	"context"
	"iter"
	"net/url"
	"strings"
	"time"
)

// JobsService submits and follows jobs.
type JobsService struct{ client *Client }

// withAutoIdempotency makes a create safe to retry: a fresh Idempotency-Key
// comes first, so one passed by the caller wins.
func (c *Client) withAutoIdempotency(opts []RequestOption) []RequestOption {
	if c.maxRetries == 0 {
		return opts
	}
	return append([]RequestOption{WithIdempotencyKey(NewIdempotencyKey())}, opts...)
}

// Create submits a job. An Idempotency-Key is generated automatically, so a
// retried request never creates a duplicate job.
func (s *JobsService) Create(ctx context.Context, params *JobCreateParams, opts ...RequestOption) (*Job, error) {
	var job Job
	if err := s.client.do(ctx, "POST", "/v1/jobs", nil, params, &job, s.client.withAutoIdempotency(opts)); err != nil {
		return nil, err
	}
	return &job, nil
}

func (p *JobListParams) values() url.Values {
	if p == nil {
		return url.Values{}
	}
	q := p.ListParams.values(nil)
	if p.Status != "" {
		q.Set("status", string(p.Status))
	}
	if p.Preset != "" {
		q.Set("preset", p.Preset)
	}
	if p.CreatedAfter != nil {
		q.Set("created_after", p.CreatedAfter.UTC().Format(time.RFC3339))
	}
	if p.CreatedBefore != nil {
		q.Set("created_before", p.CreatedBefore.UTC().Format(time.RFC3339))
	}
	for k, v := range p.Metadata {
		q.Set("metadata["+k+"]", v)
	}
	return q
}

// List returns one page of jobs, newest first.
func (s *JobsService) List(ctx context.Context, params *JobListParams, opts ...RequestOption) (*Page[Job], error) {
	return getPage[Job](ctx, s.client, "/v1/jobs", params.values(), opts)
}

// All iterates over every matching job, fetching pages as needed.
func (s *JobsService) All(ctx context.Context, params *JobListParams, opts ...RequestOption) iter.Seq2[Job, error] {
	return iterate[Job](ctx, s.client, "/v1/jobs", params.values(), opts)
}

// Get retrieves a job.
func (s *JobsService) Get(ctx context.Context, id string, opts ...RequestOption) (*Job, error) {
	return doJSON[Job](ctx, s.client, "GET", "/v1/jobs/"+seg(id), nil, opts)
}

// Cancel moves a queued, scheduled or running job to canceled.
func (s *JobsService) Cancel(ctx context.Context, id string, opts ...RequestOption) (*Job, error) {
	return doJSON[Job](ctx, s.client, "POST", "/v1/jobs/"+seg(id)+"/cancel", nil, opts)
}

// Retry starts a new attempt of a failed or canceled job, under the same id.
func (s *JobsService) Retry(ctx context.Context, id string, opts ...RequestOption) (*Job, error) {
	return doJSON[Job](ctx, s.client, "POST", "/v1/jobs/"+seg(id)+"/retry", nil, opts)
}

// Delete deletes a terminal job and its outputs.
func (s *JobsService) Delete(ctx context.Context, id string, opts ...RequestOption) error {
	return s.client.do(ctx, "DELETE", "/v1/jobs/"+seg(id), nil, nil, nil, opts)
}

// Events returns the job's timeline.
func (s *JobsService) Events(ctx context.Context, id string, opts ...RequestOption) ([]JobEvent, error) {
	return getAll[JobEvent](ctx, s.client, "/v1/jobs/"+seg(id)+"/events", nil, opts)
}

// Outputs lists the job's output files.
func (s *JobsService) Outputs(ctx context.Context, id string, opts ...RequestOption) ([]JobOutput, error) {
	return getAll[JobOutput](ctx, s.client, "/v1/jobs/"+seg(id)+"/outputs", nil, opts)
}

// OutputURL returns a short-lived signed download URL for one output.
func (s *JobsService) OutputURL(ctx context.Context, id, label string, opts ...RequestOption) (*SignedURL, error) {
	return doJSON[SignedURL](ctx, s.client, "GET", "/v1/jobs/"+seg(id)+"/outputs/"+seg(label), url.Values{"redirect": {"false"}}, opts)
}

// FileURL returns a short-lived signed URL for a file of an HLS package,
// e.g. "master.m3u8" or "720p/seg_00001.m4s".
func (s *JobsService) FileURL(ctx context.Context, id, path string, opts ...RequestOption) (*SignedURL, error) {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = seg(p)
	}
	return doJSON[SignedURL](ctx, s.client, "GET", "/v1/jobs/"+seg(id)+"/files/"+strings.Join(parts, "/"), url.Values{"redirect": {"false"}}, opts)
}

// Deliveries lists deliveries of the job's outputs to connections.
func (s *JobsService) Deliveries(ctx context.Context, id string, opts ...RequestOption) ([]Delivery, error) {
	return getAll[Delivery](ctx, s.client, "/v1/jobs/"+seg(id)+"/deliveries", nil, opts)
}

// Deliver delivers the job's outputs (again, or somewhere new); it waits for
// the job if it has not completed yet.
func (s *JobsService) Deliver(ctx context.Context, id string, destination *JobDestination, opts ...RequestOption) (*Delivery, error) {
	return doJSONBody[Delivery](ctx, s.client, "POST", "/v1/jobs/"+seg(id)+"/deliveries", destination, opts)
}

// WaitForOptions tune [JobsService.WaitFor].
type WaitForOptions struct {
	// PollInterval defaults to 2 s.
	PollInterval time.Duration
	// Timeout gives up with a [*WaitTimeoutError]; 0 waits until ctx ends.
	Timeout time.Duration
	// OnProgress is called with every polled job.
	OnProgress func(*Job)
}

// WaitFor polls until the job is completed, failed or canceled, and returns
// it. It does not return an error for a failed job: check Status.
func (s *JobsService) WaitFor(ctx context.Context, id string, opts *WaitForOptions) (*Job, error) {
	o := WaitForOptions{}
	if opts != nil {
		o = *opts
	}
	if o.PollInterval <= 0 {
		o.PollInterval = 2 * time.Second
	}
	var deadline time.Time
	if o.Timeout > 0 {
		deadline = time.Now().Add(o.Timeout)
	}
	for {
		job, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if o.OnProgress != nil {
			o.OnProgress(job)
		}
		if job.Status.IsTerminal() {
			return job, nil
		}
		if !deadline.IsZero() && time.Now().Add(o.PollInterval).After(deadline) {
			return job, &WaitTimeoutError{JobID: id, Job: job, Timeout: o.Timeout}
		}
		if err := sleep(ctx, o.PollInterval); err != nil {
			return job, err
		}
	}
}

// ProbeService probes inputs without transcoding.
type ProbeService struct{ client *Client }

// Create probes an input, as a probe-only job. With Wait it blocks up to 60 s
// for the result (and the request timeout is raised to 90 s).
func (s *ProbeService) Create(ctx context.Context, params *ProbeParams, opts ...RequestOption) (*Job, error) {
	q := url.Values{}
	if params.Wait {
		q.Set("wait", "true")
		opts = append([]RequestOption{WithRequestTimeout(90 * time.Second)}, opts...)
	}
	var job Job
	if err := s.client.do(ctx, "POST", "/v1/probe", q, params, &job, opts); err != nil {
		return nil, err
	}
	return &job, nil
}

// doJSON sends a request without a body and decodes the response.
func doJSON[T any](ctx context.Context, c *Client, method, path string, q url.Values, opts []RequestOption) (*T, error) {
	var out T
	if err := c.do(ctx, method, path, q, nil, &out, opts); err != nil {
		return nil, err
	}
	return &out, nil
}

// doJSONBody sends a JSON body and decodes the response.
func doJSONBody[T any](ctx context.Context, c *Client, method, path string, body any, opts []RequestOption) (*T, error) {
	var out T
	if err := c.do(ctx, method, path, nil, body, &out, opts); err != nil {
		return nil, err
	}
	return &out, nil
}

// getAll fetches a single-page collection.
func getAll[T any](ctx context.Context, c *Client, path string, q url.Values, opts []RequestOption) ([]T, error) {
	page, err := getPage[T](ctx, c, path, q, opts)
	if err != nil {
		return nil, err
	}
	return page.Data, nil
}
