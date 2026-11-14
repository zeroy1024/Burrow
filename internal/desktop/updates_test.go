package desktop

import (
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// useUpdateServer routes update downloads to a local TLS server for one test.
func useUpdateServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	previous := directTransport
	directTransport = server.Client().Transport.(*http.Transport)
	t.Cleanup(func() { directTransport = previous })
	return server
}

func TestUpdateSourceMirrorPrefixesGitHubDownloads(t *testing.T) {
	var path, agent string
	server := useUpdateServer(t, func(w http.ResponseWriter, r *http.Request) {
		path, agent = r.URL.Path, r.Header.Get("User-Agent")
		io.WriteString(w, "manifest")
	})
	body, err := updateSource{mirror: server.URL + "/"}.Manifest(t.Context(), false)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	data, _ := io.ReadAll(body)
	body.Close()
	want := "/https://github.com/" + updateRepository + "/releases/latest/download/update-" + runtime.GOOS + "-" + runtime.GOARCH + ".json"
	if string(data) != "manifest" || path != want || agent != "burrow-updater" {
		t.Fatalf("mirror request = %q (agent %q, body %q), want %q", path, agent, data, want)
	}
}

func TestUpdateSourceMirrorLeavesOtherHostsAlone(t *testing.T) {
	var path string
	server := useUpdateServer(t, func(w http.ResponseWriter, r *http.Request) { path = r.URL.Path })
	// Only github.com URLs are rewritten; anything else is fetched as given.
	body, err := updateSource{mirror: "https://mirror.invalid"}.Open(t.Context(), server.URL+"/asset.zip")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	body.Close()
	if path != "/asset.zip" {
		t.Fatalf("request path = %q, want /asset.zip", path)
	}
}

func TestUpdateSourceRequiresHTTPSAndSuccess(t *testing.T) {
	server := useUpdateServer(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if body, err := (updateSource{}).Open(t.Context(), "http://127.0.0.1:1/asset.zip"); err == nil {
		body.Close()
		t.Fatal("downloaded an update over plain HTTP")
	} else if !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("plain HTTP error does not name the requirement: %v", err)
	}
	if body, err := (updateSource{}).Open(t.Context(), server.URL+"/missing.json"); err == nil {
		body.Close()
		t.Fatal("a 404 response was returned as update content")
	} else if !strings.Contains(err.Error(), "404") {
		t.Fatalf("status error does not carry the response status: %v", err)
	}
}
