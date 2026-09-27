package transcdr

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPageDecoding(t *testing.T) {
	var p Page[Asset]
	must(t, json.Unmarshal([]byte(`{"object":"list","data":[{"id":"ast_1"}],"has_more":true,"next_cursor":"ast_1"}`), &p))
	if len(p.Data) != 1 || !p.HasMore || p.NextCursor != "ast_1" {
		t.Fatalf("envelope: %+v", p)
	}
	must(t, json.Unmarshal([]byte(`{"object":"list","data":[],"has_more":false,"next_cursor":null}`), &p))
	if p.Data == nil || p.HasMore || p.NextCursor != "" {
		t.Fatalf("null cursor: %+v", p)
	}
	must(t, json.Unmarshal([]byte(`{"object":"list"}`), &p))
	if p.Data == nil || len(p.Data) != 0 {
		t.Fatalf("missing data: %+v", p)
	}
	must(t, json.Unmarshal([]byte(` [{"id":"ast_a"},{"id":"ast_b"}]`), &p))
	if len(p.Data) != 2 || p.HasMore || p.NextCursor != "" {
		t.Fatalf("bare array: %+v", p)
	}
}

func TestAllFollowsCursors(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"data":[{"id":"ast_1"},{"id":"ast_2"}],"has_more":true,"next_cursor":"ast_2"}`).
		reply(200, `{"data":[{"id":"ast_3"}],"has_more":false,"next_cursor":null}`)
	var ids []string
	for a, err := range f.client().Assets.All(context.Background(), &ListParams{Limit: 2}) {
		must(t, err)
		ids = append(ids, a.ID)
	}
	if len(ids) != 3 || ids[2] != "ast_3" {
		t.Fatalf("ids = %v", ids)
	}
	reqs := f.all()
	if qv(reqs[0].Query, "limit") != "2" || qv(reqs[0].Query, "cursor") != "" {
		t.Fatalf("first query = %v", reqs[0].Query)
	}
	if qv(reqs[1].Query, "limit") != "2" || qv(reqs[1].Query, "cursor") != "ast_2" {
		t.Fatalf("second query = %v", reqs[1].Query)
	}
}

func TestAllStopsEarly(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"data":[{"id":"ast_1"},{"id":"ast_2"}],"has_more":true,"next_cursor":"ast_2"}`)
	for a, err := range f.client().Assets.All(context.Background(), nil) {
		must(t, err)
		if a.ID == "ast_1" {
			break
		}
	}
	if n := len(f.all()); n != 1 {
		t.Fatalf("fetched %d pages after break", n)
	}
}

func TestAllYieldsError(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"data":[{"id":"ast_1"}],"has_more":true,"next_cursor":"ast_1"}`).
		reply(401, `{"error":{"type":"authentication_error","message":"no"}}`)
	var got []string
	var gotErr error
	for a, err := range f.client().Assets.All(context.Background(), nil) {
		if err != nil {
			gotErr = err
			continue
		}
		got = append(got, a.ID)
	}
	if len(got) != 1 || !IsAuthentication(gotErr) {
		t.Fatalf("got %v, err %v", got, gotErr)
	}
}

func TestAllStopsWithoutCursor(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"data":[{"id":"ast_1"}],"has_more":true,"next_cursor":null}`)
	items, err := Collect(f.client().Assets.All(context.Background(), nil), 0)
	must(t, err)
	if len(items) != 1 || len(f.all()) != 1 {
		t.Fatalf("items %d, requests %d", len(items), len(f.all()))
	}
}

func TestCollect(t *testing.T) {
	f := newFakeAPI(t)
	f.reply(200, `{"data":[{"id":"a"},{"id":"b"}],"has_more":true,"next_cursor":"b"}`).
		reply(200, `{"data":[{"id":"c"}],"has_more":false}`)
	items, err := Collect(f.client().Assets.All(context.Background(), nil), 2)
	must(t, err)
	if len(items) != 2 || len(f.all()) != 1 {
		t.Fatalf("max 2: items %d, requests %d", len(items), len(f.all()))
	}
	f = newFakeAPI(t)
	f.reply(200, `{"data":[{"id":"a"}],"has_more":true,"next_cursor":"a"}`).reply(500, "").reply(500, "").reply(500, "")
	items, err = Collect(f.client().Assets.All(context.Background(), nil), 0)
	if err == nil || len(items) != 1 {
		t.Fatalf("error: items %v, err %v", items, err)
	}
}

func TestListParamsValues(t *testing.T) {
	var nilParams *ListParams
	if q := nilParams.values(nil); len(q) != 0 {
		t.Fatalf("nil = %v", q)
	}
	q := (&ListParams{Limit: 50, Cursor: "job_9"}).values(nil)
	if q.Get("limit") != "50" || q.Get("cursor") != "job_9" {
		t.Fatalf("q = %v", q)
	}
}
