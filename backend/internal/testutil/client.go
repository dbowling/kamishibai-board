package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// Client issues in-process HTTP requests against a test app.
//
// It builds the same router the real server uses, including PocketBase's
// generated collection endpoints and our custom routes, so tests exercise the
// actual API rules and hooks rather than a stand-in.
type Client struct {
	app *tests.TestApp
	mux http.Handler
}

// NewClient builds a Client. Call it after the app's hooks and routes have been
// registered, since it triggers the serve event to collect them.
func NewClient(t testing.TB, app *tests.TestApp) *Client {
	t.Helper()

	baseRouter, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("build router: %v", err)
	}

	event := new(core.ServeEvent)
	event.App = app
	event.Router = baseRouter

	// Triggering OnServe runs the handlers that register our custom routes.
	err = app.OnServe().Trigger(event, func(e *core.ServeEvent) error { return nil })
	if err != nil {
		t.Fatalf("trigger serve event: %v", err)
	}

	mux, err := event.Router.BuildMux()
	if err != nil {
		t.Fatalf("build mux: %v", err)
	}

	return &Client{app: app, mux: mux}
}

// Token issues a valid auth token for a user record.
func Token(t testing.TB, user *core.Record) string {
	t.Helper()

	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("mint auth token: %v", err)
	}
	return token
}

// Response is a decoded HTTP response.
type Response struct {
	Status int
	Body   []byte
}

// JSON decodes the response body into target.
func (r *Response) JSON(t testing.TB, target any) {
	t.Helper()

	if err := json.Unmarshal(r.Body, target); err != nil {
		t.Fatalf("decode response body: %v\nbody: %s", err, r.Body)
	}
}

// Map decodes the response body into a generic map.
func (r *Response) Map(t testing.TB) map[string]any {
	t.Helper()

	out := map[string]any{}
	r.JSON(t, &out)
	return out
}

// Do performs a request. Pass an empty token for an unauthenticated request, and
// nil body for no payload.
func (c *Client) Do(t testing.TB, method, path, token string, body any) *Response {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("content-type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	recorder := httptest.NewRecorder()
	c.mux.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	payload, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	return &Response{Status: result.StatusCode, Body: payload}
}

func (c *Client) GET(t testing.TB, path, token string) *Response {
	return c.Do(t, http.MethodGet, path, token, nil)
}

func (c *Client) POST(t testing.TB, path, token string, body any) *Response {
	return c.Do(t, http.MethodPost, path, token, body)
}

func (c *Client) PATCH(t testing.TB, path, token string, body any) *Response {
	return c.Do(t, http.MethodPatch, path, token, body)
}

func (c *Client) DELETE(t testing.TB, path, token string) *Response {
	return c.Do(t, http.MethodDelete, path, token, nil)
}
