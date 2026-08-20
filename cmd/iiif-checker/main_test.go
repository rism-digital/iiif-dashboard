package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDetectVersionAndLevel(t *testing.T) {
	doc := map[string]any{
		"@context": "http://iiif.io/api/image/3/context.json",
		"profile":  "http://iiif.io/api/image/3/level2.json",
	}
	if got := detectVersion(doc); got != "v3" {
		t.Fatalf("detectVersion() = %q", got)
	}
	if got := detectLevel(doc); got != "Level 2" {
		t.Fatalf("detectLevel() = %q", got)
	}
}

func TestRepresentativeImageURL(t *testing.T) {
	doc := map[string]any{"sizes": []any{
		map[string]any{"width": float64(800), "height": float64(600)},
		map[string]any{"width": float64(200), "height": float64(150)},
	}}
	want := "https://example.org/iiif/id/full/200,150/0/default.jpg"
	if got := representativeImageURL("https://example.org/iiif/id/info.json", doc); got != want {
		t.Fatalf("representativeImageURL() = %q", got)
	}
}

func TestExpectedDocumentIdentifier(t *testing.T) {
	manifestURL, err := url.Parse("https://example.org/iiif/manifest?version=3")
	if err != nil {
		t.Fatal(err)
	}
	if got := expectedDocumentIdentifier("presentation", manifestURL); got != manifestURL.String() {
		t.Fatalf("presentation identifier = %q, want %q", got, manifestURL.String())
	}

	infoURL, err := url.Parse("https://example.org/iiif/encoded%2Fid/info.json?format=json#fragment")
	if err != nil {
		t.Fatal(err)
	}
	if got := expectedDocumentIdentifier("image", infoURL); got != "https://example.org/iiif/encoded%2Fid" {
		t.Fatalf("image identifier = %q", got)
	}
}

func TestCheckerSetsExplicitUserAgent(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.UserAgent()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	response, err := newChecker("https://dashboard.example").request(context.Background(), http.MethodOptions, server.URL, "", true, true)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()

	if got := <-received; got != checkerUserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, checkerUserAgent)
	}
}

func TestCheckJSONValidatesIdentifierAgainstFinalResponseURL(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/ld+json")
		switch r.URL.Path {
		case "/manifest-redirect":
			http.Redirect(w, r, server.URL+"/manifest", http.StatusFound)
		case "/manifest":
			_, _ = w.Write([]byte(`{"@context":"http://iiif.io/api/presentation/3/context.json","id":"` + server.URL + `/manifest","type":"Manifest","items":[]}`))
		case "/bad-manifest":
			_, _ = w.Write([]byte(`{"@context":"http://iiif.io/api/presentation/3/context.json","id":"https://wrong.example/manifest","type":"Manifest","items":[]}`))
		case "/missing-id-manifest":
			_, _ = w.Write([]byte(`{"@context":"http://iiif.io/api/presentation/3/context.json","type":"Manifest","items":[]}`))
		case "/not-acceptable":
			w.WriteHeader(http.StatusNotAcceptable)
		case "/image-redirect/info.json":
			http.Redirect(w, r, server.URL+"/iiif/image/info.json", http.StatusFound)
		case "/iiif/image/info.json":
			_, _ = w.Write([]byte(`{"@context":"http://iiif.io/api/image/3/context.json","id":"` + server.URL + `/iiif/image","type":"ImageService3","profile":"level2","width":640,"height":480}`))
		case "/iiif/bad/info.json":
			_, _ = w.Write([]byte(`{"@context":"http://iiif.io/api/image/2/context.json","@id":"` + server.URL + `/iiif/other","profile":"http://iiif.io/api/image/2/level2.json","width":640,"height":480}`))
		case "/iiif/missing-id/info.json":
			_, _ = w.Write([]byte(`{"@context":"http://iiif.io/api/image/2/context.json","profile":"http://iiif.io/api/image/2/level2.json","width":640,"height":480}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	checker := newChecker("https://dashboard.example")
	for _, test := range []struct {
		name        string
		address     string
		kind        string
		requested   string
		wantStatus  string
		wantSummary string
	}{
		{name: "manifest follows redirect", address: server.URL + "/manifest-redirect", kind: "presentation", wantStatus: "pass", wantSummary: "id matches the final response URL"},
		{name: "manifest mismatch", address: server.URL + "/bad-manifest", kind: "presentation", requested: "v2", wantStatus: "warning", wantSummary: `Valid v3 manifest retrieved, but id is "https://wrong.example/manifest"; expected the final response URL`},
		{name: "manifest missing identifier", address: server.URL + "/missing-id-manifest", kind: "presentation", wantStatus: "fail", wantSummary: "missing the required id string"},
		{name: "identifier result remains visible with an advisory", address: server.URL + "/manifest", kind: "presentation", requested: "v2", wantStatus: "advisory", wantSummary: "id matches the final response URL"},
		{name: "unavailable representation is handled", address: server.URL + "/not-acceptable", kind: "presentation", requested: "v3", wantStatus: "pass", wantSummary: "correctly declined"},
		{name: "default representation cannot be declined", address: server.URL + "/not-acceptable", kind: "presentation", wantStatus: "fail", wantSummary: "HTTP 406"},
		{name: "image follows redirect", address: server.URL + "/image-redirect/info.json", kind: "image", wantStatus: "pass", wantSummary: "id matches the image service base URI"},
		{name: "image mismatch", address: server.URL + "/iiif/bad/info.json", kind: "image", wantStatus: "warning", wantSummary: `Valid v2 image information retrieved, but @id is "` + server.URL + `/iiif/other"; expected the image service base URI "` + server.URL + `/iiif/bad"`},
		{name: "image missing identifier", address: server.URL + "/iiif/missing-id/info.json", kind: "image", wantStatus: "fail", wantSummary: "missing the required @id string"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := checker.checkJSON(context.Background(), test.address, test.kind, test.requested).Result
			if result.Status != test.wantStatus {
				t.Fatalf("status = %q, want %q; summary: %s", result.Status, test.wantStatus, result.Summary)
			}
			if !strings.Contains(result.Summary, test.wantSummary) {
				t.Fatalf("summary = %q, want it to contain %q", result.Summary, test.wantSummary)
			}
		})
	}
}

func TestCheckCompression(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "gzip" {
			http.Error(w, "gzip was not requested", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/identity":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/corrupt":
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write([]byte("not gzip"))
		case "/gzip", "/gzip-with-multiple-vary", "/gzip-without-vary":
			w.Header().Set("Content-Encoding", "gzip")
			if r.URL.Path == "/gzip" {
				w.Header().Set("Vary", "Accept-Encoding")
			} else if r.URL.Path == "/gzip-with-multiple-vary" {
				w.Header().Add("Vary", "Origin")
				w.Header().Add("Vary", "Accept-Encoding")
			}
			writer := gzip.NewWriter(w)
			_, _ = writer.Write([]byte(`{"ok":true}`))
			_ = writer.Close()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	checker := newChecker("https://dashboard.example")
	for _, test := range []struct {
		name       string
		path       string
		wantStatus string
	}{
		{name: "gzip", path: "/gzip", wantStatus: "pass"},
		{name: "gzip with multiple vary fields", path: "/gzip-with-multiple-vary", wantStatus: "pass"},
		{name: "identity", path: "/identity", wantStatus: "advisory"},
		{name: "missing vary", path: "/gzip-without-vary", wantStatus: "warning"},
		{name: "corrupt gzip", path: "/corrupt", wantStatus: "fail"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := checker.checkCompression(context.Background(), server.URL+test.path)
			if result.Status != test.wantStatus {
				t.Fatalf("status = %q, want %q; summary: %s", result.Status, test.wantStatus, result.Summary)
			}
		})
	}
}

func TestCheckJSONRetainsRedirectResponseHeaders(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			w.Header().Add("Set-Cookie", "first=1")
			w.Header().Add("Set-Cookie", "second=2")
			w.Header().Set("X-Redirect", "first")
			w.Header().Set("Location", "/middle")
			w.WriteHeader(http.StatusFound)
		case "/middle":
			w.Header().Set("Cache-Control", "max-age=60")
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusSeeOther)
		case "/final":
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/ld+json")
			_, _ = w.Write([]byte(`{"@context":"http://iiif.io/api/presentation/3/context.json","id":"` + server.URL + `/final","type":"Manifest","items":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result := newChecker("https://dashboard.example").checkJSON(context.Background(), server.URL+"/start", "presentation", "").Result
	if result.Status != "pass" {
		t.Fatalf("status = %q; summary: %s", result.Status, result.Summary)
	}
	if len(result.RedirectChain) != 2 {
		t.Fatalf("RedirectChain = %#v", result.RedirectChain)
	}
	first, second := result.RedirectChain[0], result.RedirectChain[1]
	if first.URL != server.URL+"/start" || first.HTTPStatus != http.StatusFound || first.Location != "/middle" {
		t.Fatalf("first redirect = %#v", first)
	}
	if second.URL != server.URL+"/middle" || second.HTTPStatus != http.StatusSeeOther || second.Location != "/final" {
		t.Fatalf("second redirect = %#v", second)
	}
	for _, want := range []string{"Set-Cookie: first=1", "Set-Cookie: second=2", "X-Redirect: first"} {
		found := false
		for _, header := range first.ResponseHeaders {
			if header == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("first redirect headers = %#v; missing %q", first.ResponseHeaders, want)
		}
	}
}

func TestBaseRedirectDirectResponseIsWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("information"))
	}))
	defer server.Close()

	result := newChecker("https://dashboard.example").checkBaseRedirect(context.Background(), server.URL+"/info.json", nil)
	if result.Status != "warning" {
		t.Fatalf("status = %q, want warning; summary: %s", result.Status, result.Summary)
	}
}

func TestBaseRedirectNonRedirectResponseIsWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	result := newChecker("https://dashboard.example").checkBaseRedirect(context.Background(), server.URL+"/info.json", nil)
	if result.Status != "warning" {
		t.Fatalf("status = %q, want warning; summary: %s", result.Status, result.Summary)
	}
	if result.HTTPStatus != http.StatusNotFound {
		t.Fatalf("HTTP status = %d, want %d", result.HTTPStatus, http.StatusNotFound)
	}
}

func TestBaseRedirectAlternativeRedirectIsWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/info.json")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	result := newChecker("https://dashboard.example").checkBaseRedirect(context.Background(), server.URL+"/info.json", nil)
	if result.Status != "warning" {
		t.Fatalf("status = %q, want warning; summary: %s", result.Status, result.Summary)
	}
}

func TestBaseRedirectAcceptsInformationURIFromDeclaredIdentifier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/declared/info.json")
		w.WriteHeader(http.StatusSeeOther)
	}))
	defer server.Close()

	doc := map[string]any{
		"@context": "http://iiif.io/api/image/3/context.json",
		"id":       server.URL + "/declared",
	}
	result := newChecker("https://dashboard.example").checkBaseRedirect(context.Background(), server.URL+"/configured/info.json", doc)
	if result.Status != "pass" {
		t.Fatalf("status = %q, want pass; summary: %s", result.Status, result.Summary)
	}
}

func TestBaseRedirectUnexpectedInformationURIIsWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/other/info.json")
		w.WriteHeader(http.StatusSeeOther)
	}))
	defer server.Close()

	result := newChecker("https://dashboard.example").checkBaseRedirect(context.Background(), server.URL+"/configured/info.json", nil)
	if result.Status != "warning" {
		t.Fatalf("status = %q, want warning; summary: %s", result.Status, result.Summary)
	}
}

func TestCORSPassRejectsDuplicateAllowOrigin(t *testing.T) {
	checker := newChecker("https://dashboard.example")
	header := http.Header{}
	header.Add("Access-Control-Allow-Origin", "*")
	header.Add("Access-Control-Allow-Origin", "*")
	if checker.corsPass(header) {
		t.Fatal("duplicate Access-Control-Allow-Origin must fail")
	}
	if got := checker.corsIssue(header); got != "Access-Control-Allow-Origin was returned 2 times; browsers require exactly one value" {
		t.Fatalf("corsIssue() = %q", got)
	}
	header = http.Header{"Access-Control-Allow-Origin": []string{"*"}}
	if !checker.corsPass(header) {
		t.Fatal("single wildcard Access-Control-Allow-Origin must pass")
	}
}

func TestResponseResultStoresEachCORSHeaderAsAString(t *testing.T) {
	response := &http.Response{Header: http.Header{}}
	response.Header.Add("Access-Control-Allow-Origin", "*")
	response.Header.Add("Access-Control-Allow-Origin", "*")

	result := responseResult("fail", "duplicate header", response, "")
	want := []string{
		"access-control-allow-origin: *",
		"access-control-allow-origin: *",
	}
	if len(result.CorsHeaders) != len(want) {
		t.Fatalf("CorsHeaders = %#v", result.CorsHeaders)
	}
	for index := range want {
		if result.CorsHeaders[index] != want[index] {
			t.Fatalf("CorsHeaders[%d] = %q, want %q", index, result.CorsHeaders[index], want[index])
		}
	}
}

func TestAllResponseHeadersStoresSortedNamesAndDuplicateValues(t *testing.T) {
	response := &http.Response{Header: http.Header{}}
	response.Header.Set("Vary", "Accept")
	response.Header.Add("Set-Cookie", "first=1")
	response.Header.Add("Set-Cookie", "second=2")
	response.Header.Set("Content-Type", "application/ld+json")

	want := []string{
		"Content-Type: application/ld+json",
		"Set-Cookie: first=1",
		"Set-Cookie: second=2",
		"Vary: Accept",
	}
	got := allResponseHeaders(response)
	if len(got) != len(want) {
		t.Fatalf("allResponseHeaders() = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("allResponseHeaders()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestSelectProjects(t *testing.T) {
	projects := []Project{{ID: "one"}, {ID: "two"}, {ID: "three"}}
	for _, test := range []struct {
		name  string
		limit int
		want  []string
	}{
		{name: "zero selects all", limit: 0, want: []string{"one", "two", "three"}},
		{name: "limited selection preserves order", limit: 2, want: []string{"one", "two"}},
		{name: "oversized selection selects all", limit: 10, want: []string{"one", "two", "three"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			selected := selectProjects(projects, test.limit)
			if len(selected) != len(test.want) {
				t.Fatalf("selected %d projects, want %d", len(selected), len(test.want))
			}
			for index, want := range test.want {
				if selected[index].ID != want {
					t.Fatalf("selected[%d] = %q, want %q", index, selected[index].ID, want)
				}
			}
		})
	}
}

func TestValidateOptions(t *testing.T) {
	if err := validateOptions(-1, "", 1); err == nil {
		t.Fatal("negative n must fail")
	}
	if err := validateOptions(1, "example", 1); err == nil {
		t.Fatal("n and project must be mutually exclusive")
	}
	if err := validateOptions(1, "", 0); err == nil {
		t.Fatal("zero concurrency must fail")
	}
	if err := validateOptions(2, "", 1); err != nil {
		t.Fatalf("valid options failed: %v", err)
	}
}

func TestUncheckedProjectSerialization(t *testing.T) {
	result := uncheckedProject(Project{ID: "example", Name: "Example"})
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"checked":false`) || !strings.Contains(text, `"checks":{}`) {
		t.Fatalf("unchecked result = %s", text)
	}
	if strings.Contains(text, `"checkedAt"`) {
		t.Fatalf("unchecked result must omit checkedAt: %s", text)
	}
}

func TestProjectResultsDefaultsOldSnapshotsToChecked(t *testing.T) {
	var result ProjectResults
	if err := json.Unmarshal([]byte(`{"id":"old","name":"Old","checkedAt":"2026-01-01T00:00:00Z","checks":{}}`), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Checked {
		t.Fatal("a snapshot without checked must default to checked")
	}

	if err := json.Unmarshal([]byte(`{"id":"new","name":"New","checked":false,"checks":{}}`), &result); err != nil {
		t.Fatal(err)
	}
	if result.Checked {
		t.Fatal("an explicit unchecked snapshot must remain unchecked")
	}
}

func TestImageBaseURL(t *testing.T) {
	if got := imageBaseURL("https://example.org/iiif/id/info.json"); got != "https://example.org/iiif/id/" {
		t.Fatalf("imageBaseURL() = %q", got)
	}
}

func TestCheckProjectSupportsImageOnlyProjects(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch r.URL.Path {
		case "/iiif/image/info.json":
			w.Header().Set("Content-Type", `application/ld+json; profile="http://iiif.io/api/image/3/context.json"`)
			w.Header().Set("Vary", "Accept")
			_, _ = w.Write([]byte(`{"@context":"http://iiif.io/api/image/3/context.json","id":"` + server.URL + `/iiif/image","type":"ImageService3","profile":"level2","width":640,"height":480,"sizes":[{"width":64,"height":64}]}`))
		case "/iiif/image/":
			w.Header().Set("Location", server.URL+"/iiif/image/info.json")
			w.WriteHeader(http.StatusSeeOther)
		case "/iiif/image/full/64,64/0/default.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("image"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result := newChecker("https://dashboard.example").checkProject(context.Background(), Project{
		ID:           "image-only",
		Name:         "Image only",
		ImageInfoURL: server.URL + "/iiif/image/info.json",
	})

	for key := range result.Checks {
		if strings.HasPrefix(key, "presentation.") {
			t.Fatalf("image-only project unexpectedly produced %q", key)
		}
	}
	if result.Checks["image.default"].Status != "pass" {
		t.Fatalf("image.default = %#v", result.Checks["image.default"])
	}
	if result.Checks["image.response"].Status != "pass" {
		t.Fatalf("image.response = %#v", result.Checks["image.response"])
	}
	if result.Checks["image.compression"].Status != "advisory" {
		t.Fatalf("image.compression = %#v", result.Checks["image.compression"])
	}
	if len(result.Checks["image.default"].ResponseHeaders) == 0 {
		t.Fatal("image.default did not retain its response headers")
	}
	if len(result.Checks["image.v2"].ResponseHeaders) != 0 {
		t.Fatalf("image.v2 unexpectedly retained full response headers: %#v", result.Checks["image.v2"].ResponseHeaders)
	}
}
