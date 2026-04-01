package httpclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func newTestClient(url string, apiKey string) *Client {
	return &Client{baseURL: url, apiKey: apiKey, httpClient: http.DefaultClient, ctx: nil, versionCheck: &sync.Once{}}
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/boards" {
			t.Errorf("expected /boards, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{{"slug": "test"}})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "test-key")
	var boards []map[string]string
	if err := c.do("GET", "/boards", nil, &boards); err != nil {
		t.Fatal(err)
	}
	if len(boards) != 1 || boards[0]["slug"] != "test" {
		t.Errorf("unexpected response: %v", boards)
	}
}

func TestPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "New Board" {
			t.Errorf("expected name=New Board, got %s", body["name"])
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]string{"slug": "new"})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "test-key")
	var result map[string]string
	if err := c.do("POST", "/boards", map[string]string{"name": "New Board"}, &result); err != nil {
		t.Fatal(err)
	}
	if result["slug"] != "new" {
		t.Errorf("unexpected response: %v", result)
	}
}

func TestDelete204(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "test-key")
	if err := c.do("DELETE", "/boards/test", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestErrorDecoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		json.NewEncoder(w).Encode(map[string]string{"message": "slug is required"})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "test-key")
	err := c.do("POST", "/boards", map[string]string{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 422 {
		t.Errorf("expected status 422, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "slug is required" {
		t.Errorf("expected message 'slug is required', got %q", apiErr.Message)
	}
}

func TestNoAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("expected no auth header, got %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "")
	var result map[string]string
	if err := c.do("GET", "/health", nil, &result); err != nil {
		t.Fatal(err)
	}
}

func TestNilOutSkipsDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "")
	if err := c.do("POST", "/fire-and-forget", map[string]string{"a": "b"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestNoContentTypeWithoutBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "" {
			t.Errorf("expected no Content-Type for bodyless request, got %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "")
	var result map[string]string
	if err := c.do("GET", "/test", nil, &result); err != nil {
		t.Fatal(err)
	}
}
