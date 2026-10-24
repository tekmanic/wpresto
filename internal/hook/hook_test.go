package hook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestServer(builds *int32) *Server {
	buildFn := func(ctx context.Context, buildID string) error {
		atomic.AddInt32(builds, 1)
		time.Sleep(20 * time.Millisecond)
		return nil
	}
	return New("127.0.0.1:0", "s3cret", buildFn)
}

func TestGoLiveRequiresSecret(t *testing.T) {
	var builds int32
	s := newTestServer(&builds)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// No secret -> 401.
	resp, err := http.Post(ts.URL+"/go-live", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without secret, got %d", resp.StatusCode)
	}

	// Wrong secret -> 401.
	req, _ := http.NewRequest("POST", ts.URL+"/go-live", nil)
	req.Header.Set("X-WPresto-Secret", "wrong")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong secret, got %d", resp2.StatusCode)
	}
}

func TestGoLiveTriggersBuild(t *testing.T) {
	var builds int32
	s := newTestServer(&builds)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/go-live", nil)
	req.Header.Set("X-WPresto-Secret", "s3cret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	if body["status"] != "running" {
		t.Errorf("expected status running, got %q", body["status"])
	}
	if body["build_id"] == "" {
		t.Errorf("expected a build_id")
	}

	// Wait for the async build to finish.
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&builds) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&builds) != 1 {
		t.Errorf("expected build to run once, got %d", builds)
	}

	// Status should now be success.
	time.Sleep(30 * time.Millisecond)
	sresp, err := http.Get(ts.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	var st Status
	json.NewDecoder(sresp.Body).Decode(&st)
	sresp.Body.Close()
	if st.Status != "success" {
		t.Errorf("expected status success, got %q", st.Status)
	}
}

func TestGoLiveConflictsWhenBusy(t *testing.T) {
	// A buildFn that blocks until released.
	release := make(chan struct{})
	s := New("127.0.0.1:0", "s3cret", func(ctx context.Context, buildID string) error {
		<-release
		return nil
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	do := func() int {
		req, _ := http.NewRequest("POST", ts.URL+"/go-live", nil)
		req.Header.Set("X-WPresto-Secret", "s3cret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := do(); code != http.StatusAccepted {
		t.Fatalf("expected first 202, got %d", code)
	}
	// Second while first is still running -> 409.
	if code := do(); code != http.StatusConflict {
		t.Errorf("expected 409 while busy, got %d", code)
	}
	close(release)
}

func TestHealthz(t *testing.T) {
	var builds int32
	s := newTestServer(&builds)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from healthz, got %d", resp.StatusCode)
	}
}
