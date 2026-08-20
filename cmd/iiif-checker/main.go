package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultOrigin        = "https://rism-digital.github.io"
	checkerUserAgent     = "IIIF-Checker-Bot/1.0 (https://rism-digital.github.io/iiif-dashboard) go-http-client/1.1"
	resultsSchemaVersion = 2
)

var accepts = map[string]map[string]string{
	"presentation": {
		"v2": `application/ld+json; profile="http://iiif.io/api/presentation/2/context.json"`,
		"v3": `application/ld+json; profile="http://iiif.io/api/presentation/3/context.json"`,
	},
	"image": {
		"v2": `application/ld+json; profile="http://iiif.io/api/image/2/context.json"`,
		"v3": `application/ld+json; profile="http://iiif.io/api/image/3/context.json"`,
	},
}

type Registry struct {
	Projects []Project `json:"projects"`
}

type Project struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ManifestURL  string `json:"manifestUrl,omitempty"`
	ImageInfoURL string `json:"imageInfoUrl,omitempty"`
}

type CheckResult struct {
	Status          string        `json:"status"`
	Summary         string        `json:"summary"`
	CorsHeaders     []string      `json:"corsHeaders"`
	HTTPStatus      int           `json:"httpStatus,omitempty"`
	Detected        string        `json:"detected,omitempty"`
	ContentType     string        `json:"contentType,omitempty"`
	Location        string        `json:"location,omitempty"`
	RequestAccept   string        `json:"requestAccept,omitempty"`
	ResponseHeaders []string      `json:"responseHeaders,omitempty"`
	RedirectChain   []RedirectHop `json:"redirectChain,omitempty"`
}

type RedirectHop struct {
	URL             string   `json:"url"`
	HTTPStatus      int      `json:"httpStatus"`
	Location        string   `json:"location,omitempty"`
	ResponseHeaders []string `json:"responseHeaders"`
}

type ProjectResults struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Checked   bool                   `json:"checked"`
	CheckedAt string                 `json:"checkedAt,omitempty"`
	Checks    map[string]CheckResult `json:"checks"`
}

func (p *ProjectResults) UnmarshalJSON(data []byte) error {
	type projectResults ProjectResults
	var decoded projectResults
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, present := fields["checked"]; !present {
		decoded.Checked = true
	}
	*p = ProjectResults(decoded)
	return nil
}

type ResultsFile struct {
	SchemaVersion int              `json:"schemaVersion"`
	GeneratedAt   string           `json:"generatedAt"`
	Projects      []ProjectResults `json:"projects"`
}

type jsonCheck struct {
	Result   CheckResult
	Document map[string]any
}

type Checker struct {
	origin string
	client *http.Client
}

func newChecker(origin string) *Checker {
	return &Checker{origin: origin, client: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Checker) request(ctx context.Context, method, address, accept string, preflight bool, follow bool) (*http.Response, error) {
	return c.requestWithHeaders(ctx, method, address, accept, preflight, follow, nil)
}

func (c *Checker) requestWithHeaders(ctx context.Context, method, address, accept string, preflight bool, follow bool, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", checkerUserAgent)
	req.Header.Set("Origin", c.origin)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if preflight {
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "Accept")
	}
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	if follow {
		return c.client.Do(req)
	}
	client := *c.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client.Do(req)
}

func responseResult(status, summary string, resp *http.Response, detected string) CheckResult {
	r := CheckResult{Status: status, Summary: summary, CorsHeaders: []string{}, Detected: detected}
	if resp == nil {
		return r
	}
	r.HTTPStatus = resp.StatusCode
	r.ContentType = resp.Header.Get("Content-Type")
	r.Location = resp.Header.Get("Location")
	r.RedirectChain = redirectChain(resp)
	keys := make([]string, 0)
	for key := range resp.Header {
		if strings.HasPrefix(strings.ToLower(key), "access-control-") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range resp.Header.Values(key) {
			r.CorsHeaders = append(r.CorsHeaders, strings.ToLower(key)+": "+value)
		}
	}
	return r
}

func redirectChain(resp *http.Response) []RedirectHop {
	if resp == nil || resp.Request == nil {
		return nil
	}
	reversed := []RedirectHop{}
	for request := resp.Request; request != nil && request.Response != nil; request = request.Response.Request {
		redirect := request.Response
		address := ""
		if redirect.Request != nil && redirect.Request.URL != nil {
			address = redirect.Request.URL.String()
		}
		reversed = append(reversed, RedirectHop{
			URL:             address,
			HTTPStatus:      redirect.StatusCode,
			Location:        redirect.Header.Get("Location"),
			ResponseHeaders: allResponseHeaders(redirect),
		})
	}
	chain := make([]RedirectHop, len(reversed))
	for index := range reversed {
		chain[len(reversed)-1-index] = reversed[index]
	}
	return chain
}

func allResponseHeaders(resp *http.Response) []string {
	if resp == nil {
		return nil
	}
	keys := make([]string, 0, len(resp.Header))
	for key := range resp.Header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := []string{}
	for _, key := range keys {
		for _, value := range resp.Header.Values(key) {
			lines = append(lines, key+": "+value)
		}
	}
	return lines
}

func addDefaultResponseHeaders(result CheckResult, resp *http.Response, requested string) CheckResult {
	if requested == "" {
		result.ResponseHeaders = allResponseHeaders(resp)
	}
	return result
}

func (c *Checker) corsPass(headers http.Header) bool {
	return c.corsIssue(headers) == ""
}

func (c *Checker) corsIssue(headers http.Header) string {
	values := headers.Values("Access-Control-Allow-Origin")
	if len(values) == 0 {
		return "Access-Control-Allow-Origin is missing"
	}
	if len(values) > 1 {
		return fmt.Sprintf("Access-Control-Allow-Origin was returned %d times; browsers require exactly one value", len(values))
	}
	if values[0] != "*" && values[0] != c.origin {
		return fmt.Sprintf("Access-Control-Allow-Origin is %q, which does not permit %s", values[0], c.origin)
	}
	return ""
}

func detectVersion(doc map[string]any) string {
	var contexts []string
	switch value := doc["@context"].(type) {
	case string:
		contexts = []string{value}
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok {
				contexts = append(contexts, text)
			}
		}
	}
	for _, context := range contexts {
		if strings.Contains(context, "/3/context.json") {
			return "v3"
		}
	}
	for _, context := range contexts {
		if strings.Contains(context, "/2/context.json") {
			return "v2"
		}
	}
	return ""
}

func detectLevel(doc map[string]any) string {
	values := []any{doc["profile"]}
	if list, ok := doc["profile"].([]any); ok {
		values = list
	}
	for _, item := range values {
		text, ok := item.(string)
		if !ok {
			if object, objectOK := item.(map[string]any); objectOK {
				text, ok = object["profile"].(string)
			}
		}
		if !ok {
			continue
		}
		for level := 0; level <= 2; level++ {
			if strings.Contains(text, fmt.Sprintf("level%d", level)) {
				return fmt.Sprintf("Level %d", level)
			}
		}
	}
	return ""
}

func structureIssue(doc map[string]any, kind, version string) string {
	if kind == "presentation" {
		if version == "v3" {
			resourceType, hasType := doc["type"].(string)
			if !hasType || resourceType == "" {
				return `IIIF Presentation API v3 requires the top-level type string "Manifest", but it is missing.`
			}
			if resourceType != "Manifest" {
				return fmt.Sprintf(`IIIF Presentation API v3 requires the top-level type to be "Manifest"; received %q. Type values are case-sensitive.`, resourceType)
			}
			if _, ok := doc["items"].([]any); !ok {
				return `IIIF Presentation API v3 requires a top-level items array, but it is missing or is not an array.`
			}
			return ""
		}
		if version == "v2" {
			resourceType, hasType := doc["@type"].(string)
			if !hasType || resourceType == "" {
				return `IIIF Presentation API v2 requires the top-level @type string "sc:Manifest", but it is missing.`
			}
			if resourceType != "sc:Manifest" {
				return fmt.Sprintf(`IIIF Presentation API v2 requires the top-level @type to be "sc:Manifest"; received %q. Type values are case-sensitive.`, resourceType)
			}
			if _, ok := doc["sequences"].([]any); !ok {
				return `IIIF Presentation API v2 requires a top-level sequences array, but it is missing or is not an array.`
			}
			return ""
		}
		return "The JSON does not declare a recognized IIIF Presentation API context."
	}
	if _, ok := doc["width"].(float64); !ok {
		return "IIIF image information requires a numeric top-level width, but it is missing or is not a number."
	}
	if _, ok := doc["height"].(float64); !ok {
		return "IIIF image information requires a numeric top-level height, but it is missing or is not a number."
	}
	return ""
}

func identifierProperty(version string) string {
	if version == "v2" {
		return "@id"
	}
	return "id"
}

func expectedDocumentIdentifier(kind string, responseURL *url.URL) string {
	if responseURL == nil {
		return ""
	}
	if kind == "presentation" {
		return responseURL.String()
	}

	base := *responseURL
	base.RawQuery = ""
	base.ForceQuery = false
	base.Fragment = ""
	base.Path = strings.TrimSuffix(base.Path, "/info.json")
	if base.RawPath != "" {
		base.RawPath = strings.TrimSuffix(base.RawPath, "/info.json")
	}
	return strings.TrimSuffix(base.String(), "/")
}

func headerContains(header http.Header, name, target string) bool {
	for _, value := range header.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), target) || strings.TrimSpace(part) == "*" {
				return true
			}
		}
	}
	return false
}

func (c *Checker) checkJSON(ctx context.Context, address, kind, requested string) jsonCheck {
	accept := ""
	if requested != "" {
		accept = accepts[kind][requested]
	}
	resp, err := c.request(ctx, http.MethodGet, address, accept, false, true)
	if err != nil {
		result := responseResult("fail", "Request failed: "+err.Error(), nil, "")
		result.RequestAccept = accept
		return jsonCheck{Result: addDefaultResponseHeaders(result, nil, requested)}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		status, summary := "fail", fmt.Sprintf("The server returned HTTP %d.", resp.StatusCode)
		if resp.StatusCode == http.StatusNotAcceptable && requested != "" {
			status, summary = "pass", "The server correctly declined the unavailable representation (406 Not Acceptable)."
		}
		result := responseResult(status, summary, resp, "")
		result.RequestAccept = accept
		return jsonCheck{Result: addDefaultResponseHeaders(result, resp, requested)}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result := responseResult("fail", "Could not read response: "+err.Error(), resp, "")
		result.RequestAccept = accept
		return jsonCheck{Result: addDefaultResponseHeaders(result, resp, requested)}
	}
	doc := map[string]any{}
	if err := json.Unmarshal(body, &doc); err != nil {
		result := responseResult("fail", "Response was not valid JSON: "+err.Error(), resp, "")
		result.RequestAccept = accept
		return jsonCheck{Result: addDefaultResponseHeaders(result, resp, requested)}
	}
	version, level := detectVersion(doc), ""
	if kind == "image" {
		level = detectLevel(doc)
	}
	detected := strings.Trim(strings.Join([]string{version, level}, " · "), " ·")
	status, noun := "pass", "manifest"
	if kind == "image" {
		noun = "image information"
	}
	identifierName := identifierProperty(version)
	identifier, hasIdentifier := doc[identifierName].(string)
	expectedIdentifier := expectedDocumentIdentifier(kind, resp.Request.URL)
	identifierTarget := "final response URL"
	if kind == "image" {
		identifierTarget = "image service base URI"
	}
	summary := fmt.Sprintf("Valid %s %s; %s matches the %s.", detected, noun, identifierName, identifierTarget)
	if issue := structureIssue(doc, kind, version); issue != "" {
		status, summary = "fail", issue
	} else if !hasIdentifier || identifier == "" {
		status, summary = "fail", fmt.Sprintf("The IIIF %s is missing the required %s string.", noun, identifierName)
	} else if identifier != expectedIdentifier {
		status = "warning"
		summary = fmt.Sprintf("Valid %s %s retrieved, but %s is %q; expected the %s %q.", version, noun, identifierName, identifier, identifierTarget, expectedIdentifier)
	} else if !c.corsPass(resp.Header) {
		status, summary = "warning", "The representation is valid, but its CORS response is invalid: "+c.corsIssue(resp.Header)+"."
	} else if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		status, summary = "warning", "The representation is valid, but Content-Type is not a JSON media type."
	} else if kind == "image" && level == "" {
		status, summary = "warning", "Image information is valid, but no recognized compliance level is declared."
	} else if requested != "" && version != requested {
		status, summary = "advisory", fmt.Sprintf("Requested %s but received %s; supporting the requested representation through content negotiation is recommended.", requested, version)
	} else if requested != "" && !headerContains(resp.Header, "Vary", "Accept") {
		status, summary = "warning", fmt.Sprintf("The requested %s representation was returned, but Vary: Accept is missing and shared caches may serve the wrong version.", requested)
	}
	if (status == "warning" || status == "advisory") && hasIdentifier && identifier == expectedIdentifier {
		summary += fmt.Sprintf(" The %s matches the %s.", identifierName, identifierTarget)
	}
	result := responseResult(status, summary, resp, detected)
	result.RequestAccept = accept
	result = addDefaultResponseHeaders(result, resp, requested)
	return jsonCheck{Result: result, Document: doc}
}

func (c *Checker) checkCompression(ctx context.Context, address string) CheckResult {
	headers := http.Header{"Accept-Encoding": []string{"gzip"}}
	resp, err := c.requestWithHeaders(ctx, http.MethodGet, address, "", false, true, headers)
	if err != nil {
		return responseResult("fail", "Gzip request failed: "+err.Error(), nil, "")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseResult("fail", fmt.Sprintf("Gzip request returned HTTP %d.", resp.StatusCode), resp, "")
	}

	encoding := strings.TrimSpace(strings.ToLower(resp.Header.Get("Content-Encoding")))
	if encoding == "" || encoding == "identity" {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return responseResult("fail", "Could not read the identity response: "+err.Error(), resp, "")
		}
		if !json.Valid(body) {
			return responseResult("fail", "The identity response was not valid JSON.", resp, "")
		}
		return responseResult("advisory", "A usable identity response was returned; gzip compression is recommended.", resp, "")
	}
	if encoding != "gzip" {
		return responseResult("fail", fmt.Sprintf("The server returned unsupported Content-Encoding %q after gzip was requested.", encoding), resp, "")
	}

	reader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return responseResult("fail", "The response declared gzip compression but could not be decompressed: "+err.Error(), resp, "")
	}
	body, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return responseResult("fail", "The gzip response could not be decompressed: "+readErr.Error(), resp, "")
	}
	if closeErr != nil {
		return responseResult("fail", "The gzip response could not be closed cleanly: "+closeErr.Error(), resp, "")
	}
	if !json.Valid(body) {
		return responseResult("fail", "The decompressed response was not valid JSON.", resp, "")
	}
	if !headerContains(resp.Header, "Vary", "Accept-Encoding") {
		return responseResult("warning", "The response was gzip-compressed, but Vary: Accept-Encoding is missing and shared caches may serve the wrong encoding.", resp, "")
	}
	return responseResult("pass", "The response was gzip-compressed and decompressed successfully.", resp, "")
}

func (c *Checker) checkPreflight(ctx context.Context, address string) CheckResult {
	resp, err := c.request(ctx, http.MethodOptions, address, "", true, true)
	if err != nil {
		return responseResult("fail", "OPTIONS preflight failed: "+err.Error(), nil, "")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseResult("fail", fmt.Sprintf("OPTIONS preflight returned HTTP %d.", resp.StatusCode), resp, "")
	}
	if !c.corsPass(resp.Header) {
		return responseResult("fail", "OPTIONS responded, but its CORS response is invalid: "+c.corsIssue(resp.Header)+".", resp, "")
	}
	if !headerContains(resp.Header, "Access-Control-Allow-Methods", "GET") {
		return responseResult("warning", "OPTIONS succeeded, but Access-Control-Allow-Methods does not include GET.", resp, "")
	}
	if !headerContains(resp.Header, "Access-Control-Allow-Headers", "Accept") {
		return responseResult("warning", "OPTIONS succeeded, but Access-Control-Allow-Headers does not include Accept.", resp, "")
	}
	return responseResult("pass", "OPTIONS preflight permits GET with the Accept request header.", resp, "")
}

func representativeImageURL(infoAddress string, doc map[string]any) string {
	suffix := "256,"
	if sizes, ok := doc["sizes"].([]any); ok {
		type size struct{ w, h int }
		list := []size{}
		for _, raw := range sizes {
			if object, ok := raw.(map[string]any); ok {
				w, wok := object["width"].(float64)
				h, hok := object["height"].(float64)
				if wok && hok {
					list = append(list, size{int(w), int(h)})
				}
			}
		}
		sort.Slice(list, func(i, j int) bool { return list[i].w < list[j].w })
		if len(list) > 0 {
			suffix = fmt.Sprintf("%d,%d", list[0].w, list[0].h)
		}
	}
	base := strings.TrimSuffix(infoAddress, "/info.json")
	return base + "/full/" + suffix + "/0/default.jpg"
}

func (c *Checker) checkImage(ctx context.Context, address string) CheckResult {
	resp, err := c.request(ctx, http.MethodGet, address, "", false, true)
	if err != nil {
		return responseResult("fail", "Representative image request failed: "+err.Error(), nil, "")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseResult("fail", fmt.Sprintf("Representative image request returned HTTP %d.", resp.StatusCode), resp, "")
	}
	if !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "image/") {
		return responseResult("warning", "Representative request succeeded, but Content-Type is not an image media type.", resp, "")
	}
	if !c.corsPass(resp.Header) {
		return responseResult("warning", "Image was delivered, but its CORS response is invalid: "+c.corsIssue(resp.Header)+".", resp, "")
	}
	return responseResult("pass", "Representative image request succeeded with an image response and usable CORS.", resp, "")
}

func imageBaseURL(infoAddress string) string {
	return strings.TrimSuffix(infoAddress, "/info.json") + "/"
}

func declaredImageInfoURL(doc map[string]any) string {
	version := detectVersion(doc)
	identifier, ok := doc[identifierProperty(version)].(string)
	if !ok || identifier == "" {
		return ""
	}
	return strings.TrimSuffix(identifier, "/") + "/info.json"
}

func (c *Checker) checkBaseRedirect(ctx context.Context, infoAddress string, doc map[string]any) CheckResult {
	base := imageBaseURL(infoAddress)
	resp, err := c.request(ctx, http.MethodGet, base, "application/ld+json", false, false)
	if err != nil {
		return responseResult("fail", "Base URI redirect check failed: "+err.Error(), nil, "")
	}
	defer resp.Body.Close()
	location := resp.Header.Get("Location")
	resolved := ""
	if parsedBase, err := url.Parse(base); err == nil {
		if parsed, err := url.Parse(location); err == nil {
			resolved = parsedBase.ResolveReference(parsed).String()
		}
	}
	if resp.StatusCode == 303 {
		if resolved == "" {
			return responseResult("fail", "Base URI returned HTTP 303 without a usable Location header.", resp, "")
		}
		declaredInfoAddress := declaredImageInfoURL(doc)
		if resolved == infoAddress || (declaredInfoAddress != "" && resolved == declaredInfoAddress) {
			return responseResult("pass", "Base URI returned HTTP 303 to "+resolved+".", resp, "")
		}
		return responseResult("warning", fmt.Sprintf("Base URI returned HTTP 303 to %s, which differs from both the checked and declared image information URIs.", resolved), resp, "")
	}
	if resp.StatusCode == 200 {
		return responseResult("warning", "Base URI returned HTTP 200 directly; IIIF recommends a 303 redirect to info.json.", resp, "")
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return responseResult("warning", fmt.Sprintf("Base URI redirected with HTTP %d to %s; IIIF recommends 303.", resp.StatusCode, resolved), resp, "")
	}
	return responseResult("warning", fmt.Sprintf("Base URI returned HTTP %d instead of the recommended 303 redirect to info.json.", resp.StatusCode), resp, "")
}

func (c *Checker) checkProject(ctx context.Context, project Project) ProjectResults {
	checks := map[string]CheckResult{}
	if project.ManifestURL != "" {
		checks["presentation.default"] = c.checkJSON(ctx, project.ManifestURL, "presentation", "").Result
		checks["presentation.compression"] = c.checkCompression(ctx, project.ManifestURL)
		checks["presentation.v2"] = c.checkJSON(ctx, project.ManifestURL, "presentation", "v2").Result
		checks["presentation.v3"] = c.checkJSON(ctx, project.ManifestURL, "presentation", "v3").Result
		checks["presentation.preflight"] = c.checkPreflight(ctx, project.ManifestURL)
	}
	if project.ImageInfoURL != "" {
		info := c.checkJSON(ctx, project.ImageInfoURL, "image", "")
		checks["image.default"] = info.Result
		checks["image.compression"] = c.checkCompression(ctx, project.ImageInfoURL)
		checks["image.v2"] = c.checkJSON(ctx, project.ImageInfoURL, "image", "v2").Result
		checks["image.v3"] = c.checkJSON(ctx, project.ImageInfoURL, "image", "v3").Result
		checks["image.info-preflight"] = c.checkPreflight(ctx, project.ImageInfoURL)
		checks["image.base-redirect"] = c.checkBaseRedirect(ctx, project.ImageInfoURL, info.Document)
		if info.Document != nil {
			imageURL := representativeImageURL(project.ImageInfoURL, info.Document)
			checks["image.response"] = c.checkImage(ctx, imageURL)
			checks["image.response-preflight"] = c.checkPreflight(ctx, imageURL)
		} else {
			checks["image.response"] = responseResult("unknown", "Representative image was not requested because default info.json could not be parsed.", nil, "")
			checks["image.response-preflight"] = responseResult("unknown", "OPTIONS was not requested because the representative image URL could not be derived.", nil, "")
		}
	}
	return ProjectResults{ID: project.ID, Name: project.Name, Checked: true, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano), Checks: checks}
}

func uncheckedProject(project Project) ProjectResults {
	return ProjectResults{ID: project.ID, Name: project.Name, Checked: false, Checks: map[string]CheckResult{}}
}

func selectProjects(projects []Project, limit int) []Project {
	if limit == 0 || limit >= len(projects) {
		return projects
	}
	return projects[:limit]
}

func validateOptions(limit int, projectID string, concurrency int) error {
	if concurrency < 1 {
		return errors.New("concurrency must be at least 1")
	}
	if limit < 0 {
		return errors.New("n must be zero or greater")
	}
	if limit > 0 && projectID != "" {
		return errors.New("n and project cannot be used together")
	}
	return nil
}

func run() error {
	projectsPath := flag.String("projects", "projects.json", "project registry path")
	resultsPath := flag.String("results", "results.json", "result output path")
	origin := flag.String("origin", envOr("DASHBOARD_ORIGIN", defaultOrigin), "Origin used for CORS checks")
	projectID := flag.String("project", "", "check only this project ID and preserve other stored results")
	limit := flag.Int("n", 0, "maximum number of projects to check; 0 checks all projects")
	concurrency := flag.Int("concurrency", 6, "maximum number of projects checked concurrently")
	flag.Parse()
	if err := validateOptions(*limit, *projectID, *concurrency); err != nil {
		return err
	}
	data, err := os.ReadFile(*projectsPath)
	if err != nil {
		return err
	}
	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return err
	}
	checker := newChecker(*origin)
	output := ResultsFile{SchemaVersion: resultsSchemaVersion, Projects: []ProjectResults{}}
	if *projectID != "" {
		if existing, err := loadResults(*resultsPath, registry.Projects); err == nil {
			output = existing
		}
	} else {
		for _, project := range registry.Projects {
			output.Projects = append(output.Projects, uncheckedProject(project))
		}
	}
	selected := []Project{}
	if *projectID != "" {
		for _, project := range registry.Projects {
			if project.ID == *projectID {
				selected = append(selected, project)
			}
		}
	} else {
		selected = selectProjects(registry.Projects, *limit)
	}
	if *projectID != "" && len(selected) == 0 {
		return fmt.Errorf("project ID %q was not found", *projectID)
	}
	if *limit > 0 {
		fmt.Printf("Checking %d of %d projects; %d will be marked not checked.\n", len(selected), len(registry.Projects), len(registry.Projects)-len(selected))
	}
	type completedProject struct {
		index  int
		result ProjectResults
	}
	projectNames := make([]string, len(selected))
	for index, project := range selected {
		projectNames[index] = project.Name
	}
	progress := newProjectProgress(os.Stdout, projectNames, supportsInteractiveProgress(os.Stdout))
	configureTerminalProgress(progress, os.Stdout)
	progress.Start()
	completed := make(chan completedProject, len(selected))
	semaphore := make(chan struct{}, *concurrency)
	for index, project := range selected {
		go func(index int, project Project) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			progress.MarkRunning(index)
			result := checker.checkProject(context.Background(), project)
			progress.MarkFinished(index)
			completed <- completedProject{index: index, result: result}
		}(index, project)
	}
	fresh := make([]ProjectResults, len(selected))
	for range selected {
		item := <-completed
		fresh[item.index] = item.result
	}
	progress.Close()
	for _, checked := range fresh {
		replaced := false
		for index := range output.Projects {
			if output.Projects[index].ID == checked.ID {
				output.Projects[index] = checked
				replaced = true
				break
			}
		}
		if !replaced {
			output.Projects = append(output.Projects, checked)
		}
	}
	output.GeneratedAt = time.Now().UTC().Format(time.RFC3339Nano)
	output.SchemaVersion = resultsSchemaVersion
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*resultsPath), 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return os.WriteFile(*resultsPath, append(encoded, '\n'), 0o644)
}

func loadResults(path string, projects []Project) (ResultsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ResultsFile{}, err
	}
	var envelope struct {
		SchemaVersion int             `json:"schemaVersion"`
		GeneratedAt   string          `json:"generatedAt"`
		Projects      json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ResultsFile{}, err
	}
	output := ResultsFile{SchemaVersion: envelope.SchemaVersion, GeneratedAt: envelope.GeneratedAt, Projects: []ProjectResults{}}
	if len(envelope.Projects) > 0 && envelope.Projects[0] == '[' {
		if err := json.Unmarshal(envelope.Projects, &output.Projects); err != nil {
			return ResultsFile{}, err
		}
		return output, nil
	}
	legacy := map[string]ProjectResults{}
	if err := json.Unmarshal(envelope.Projects, &legacy); err != nil {
		return ResultsFile{}, err
	}
	byID := map[string]Project{}
	for _, project := range projects {
		byID[project.ID] = project
	}
	for _, project := range projects {
		if result, ok := legacy[project.ID]; ok {
			result.ID = project.ID
			result.Name = project.Name
			output.Projects = append(output.Projects, result)
		}
	}
	return output, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
