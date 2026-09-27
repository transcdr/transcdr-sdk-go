package transcdr

import (
	"bytes"
	"context"
	"encoding/json"
	"iter"
	"net/url"
	"strconv"
)

// ListParams page through a cursor-paginated list.
type ListParams struct {
	// Limit is 1–100; the API's default is 20.
	Limit int
	// Cursor is the NextCursor of the previous page.
	Cursor string
}

func (p *ListParams) values(q url.Values) url.Values {
	if q == nil {
		q = url.Values{}
	}
	if p == nil {
		return q
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	return q
}

// Page is one page of a list, newest first.
type Page[T any] struct {
	Data    []T  `json:"data"`
	HasMore bool `json:"has_more"`
	// NextCursor fetches the next page; "" on the last one.
	NextCursor string `json:"next_cursor"`
}

// UnmarshalJSON accepts the list envelope, and also a bare array.
func (p *Page[T]) UnmarshalJSON(b []byte) error {
	if t := bytes.TrimSpace(b); len(t) > 0 && t[0] == '[' {
		p.HasMore, p.NextCursor = false, ""
		return json.Unmarshal(t, &p.Data)
	}
	var raw struct {
		Data       []T     `json:"data"`
		HasMore    bool    `json:"has_more"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	p.Data, p.HasMore, p.NextCursor = raw.Data, raw.HasMore, ""
	if raw.NextCursor != nil {
		p.NextCursor = *raw.NextCursor
	}
	if p.Data == nil {
		p.Data = []T{}
	}
	return nil
}

func getPage[T any](ctx context.Context, c *Client, path string, q url.Values, opts []RequestOption) (*Page[T], error) {
	var page Page[T]
	if err := c.do(ctx, "GET", path, q, nil, &page, opts); err != nil {
		return nil, err
	}
	return &page, nil
}

// iterate walks every page from the one q asks for, yielding each item. It
// stops at the first error, which it yields.
func iterate[T any](ctx context.Context, c *Client, path string, q url.Values, opts []RequestOption) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		q := cloneValues(q)
		for {
			page, err := getPage[T](ctx, c, path, q, opts)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range page.Data {
				if !yield(item, nil) {
					return
				}
			}
			if !page.HasMore || page.NextCursor == "" {
				return
			}
			q.Set("cursor", page.NextCursor)
		}
	}
}

// Collect gathers a paginated iterator into a slice, stopping after max items
// when max > 0.
func Collect[T any](seq iter.Seq2[T, error], max int) ([]T, error) {
	var out []T
	for item, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, item)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out, nil
}

func cloneValues(q url.Values) url.Values {
	out := url.Values{}
	for k, vs := range q {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
