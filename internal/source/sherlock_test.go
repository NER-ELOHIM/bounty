package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSherlockContests(t *testing.T) {
	var status string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status = r.URL.Query().Get("status")
		_, _ = w.Write([]byte(`{"items":[
		  {"id":1260,"title":"XRP Ledger","short_description":"ledger audit","type_label":"Public Bug Bounty",
		   "private":false,"prize_pool":519000,"token":"RLUSD","starts_at":1776067200,"ends_at":1777276800},
		  {"id":1300,"title":"Invite only","private":true,"prize_pool":18500,"token":"USDC","starts_at":1,"ends_at":2}]}`))
	}))
	t.Cleanup(srv.Close)

	s := NewSherlock(5 * time.Second)
	s.Base = srv.URL

	got, err := s.Contests(context.Background(), "RUNNING")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "RUNNING" {
		t.Errorf("status sent = %q", status)
	}
	if len(got) != 1 {
		t.Fatalf("expected the private contest to be left out, got %d", len(got))
	}
	c := got[0]
	if c.ID != 1260 || c.PrizePool != 519000 || c.Token != "RLUSD" || c.Kind != "Public Bug Bounty" {
		t.Errorf("Contest = %+v", c)
	}
	if !c.StartsAt.Equal(time.Unix(1776067200, 0)) || !c.EndsAt.Equal(time.Unix(1777276800, 0)) {
		t.Errorf("dates = %v, %v", c.StartsAt, c.EndsAt)
	}
	if c.URL() != "https://audits.sherlock.xyz/contests/1260" {
		t.Errorf("URL = %q", c.URL())
	}
}

func TestSherlockFalhaForaDe200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	s := NewSherlock(5 * time.Second)
	s.Base = srv.URL
	if _, err := s.Contests(context.Background(), "RUNNING"); err == nil {
		t.Fatal("expected an error for HTTP 502")
	}
}
