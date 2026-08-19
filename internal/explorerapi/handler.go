package explorerapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"

	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

const maxHTTPResponseBytes = 1 << 20

type HandlerConfig struct {
	ProductVersion string
	AllowedHosts   []string
	Web            http.Handler
}

type handler struct {
	service        *Service
	productVersion string
	allowedHosts   map[string]struct{}
	web            http.Handler
	schema         json.RawMessage
}

// NewHandler returns a Host-locked read-only Explorer handler.
func NewHandler(service *Service, config HandlerConfig) (http.Handler, error) {
	if service == nil || config.Web == nil || len(config.AllowedHosts) == 0 {
		return nil, fmt.Errorf("invalid Explorer handler configuration")
	}
	schema, _, err := explorercontract.Generated()
	if err != nil {
		return nil, err
	}
	h := &handler{
		service: service, productVersion: config.ProductVersion, web: config.Web,
		allowedHosts: map[string]struct{}{}, schema: schema,
	}
	for _, host := range config.AllowedHosts {
		h.allowedHosts[strings.ToLower(strings.TrimSpace(host))] = struct{}{}
	}
	return h, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w.Header())
	if _, ok := h.allowedHosts[strings.ToLower(r.Host)]; !ok {
		h.writeError(w, r, &QueryError{Code: "host_rejected", Summary: "The request Host is not allowed.", Status: http.StatusForbidden})
		return
	}
	if len(r.RequestURI) > 4096 {
		h.writeError(w, r, &QueryError{Code: "request_too_large", Summary: "The request target is too large.", Status: http.StatusRequestURITooLong})
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		h.writeError(w, r, &QueryError{Code: "method_not_allowed", Summary: "Explorer is read-only.", Status: http.StatusMethodNotAllowed})
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		h.web.ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		h.writeError(w, r, &QueryError{Code: "method_not_allowed", Summary: "This API endpoint accepts GET only.", Status: http.StatusMethodNotAllowed})
		return
	}

	switch {
	case r.URL.Path == "/api/v1/status":
		if !h.allowQuery(w, r) {
			return
		}
		h.writeData(w, r, h.service.Status(h.productVersion), explorercontract.Meta{Truncated: false})
	case r.URL.Path == "/api/v1/entities":
		h.entities(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/entities/"):
		h.entity(w, r)
	case r.URL.Path == "/api/v1/search":
		h.search(w, r)
	case r.URL.Path == "/api/v1/graph":
		h.graph(w, r)
	case r.URL.Path == "/api/v1/health":
		h.health(w, r)
	case r.URL.Path == "/api/v1/schema":
		if !h.allowQuery(w, r) {
			return
		}
		h.writeData(w, r, h.schema, explorercontract.Meta{Truncated: false})
	default:
		h.writeError(w, r, &QueryError{Code: "not_found", Summary: "The requested Explorer endpoint was not found.", Status: http.StatusNotFound})
	}
}

func (h *handler) entities(w http.ResponseWriter, r *http.Request) {
	if !h.allowQuery(w, r, "q", "kind", "status", "surface", "tag", "limit", "cursor", "sort", "direction") {
		return
	}
	limit, err := parseInt(r.URL.Query().Get("limit"), 50)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	page, meta, err := h.service.List(ListOptions{
		Query: r.URL.Query().Get("q"), Kinds: values(r, "kind"), Statuses: values(r, "status"),
		Surfaces: values(r, "surface"), Tags: values(r, "tag"), Limit: limit,
		Cursor: r.URL.Query().Get("cursor"), Sort: r.URL.Query().Get("sort"), Direction: r.URL.Query().Get("direction"),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeData(w, r, page, meta)
}

func (h *handler) entity(w http.ResponseWriter, r *http.Request) {
	if !h.allowQuery(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/entities/")
	if id == "" || strings.Contains(id, "/") {
		h.writeError(w, r, usageError("invalid_entity_id", "The entity ID is invalid."))
		return
	}
	detail, meta, err := h.service.EntityDetail(id, 200)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeData(w, r, detail, meta)
}

func (h *handler) search(w http.ResponseWriter, r *http.Request) {
	if !h.allowQuery(w, r, "q", "kind", "status", "surface", "tag", "limit") {
		return
	}
	limit, err := parseInt(r.URL.Query().Get("limit"), 20)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	page, meta, err := h.service.Search(SearchOptions{
		Query: r.URL.Query().Get("q"), Kinds: values(r, "kind"), Statuses: values(r, "status"),
		Surfaces: values(r, "surface"), Tags: values(r, "tag"), Limit: limit,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeData(w, r, page, meta)
}

func (h *handler) graph(w http.ResponseWriter, r *http.Request) {
	if !h.allowQuery(w, r, "mode", "group_by", "group", "id", "depth") {
		return
	}
	depth, err := parseInt(r.URL.Query().Get("depth"), 0)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	page, meta, err := h.service.Graph(GraphOptions{
		Mode: explorercontract.GraphMode(r.URL.Query().Get("mode")), GroupBy: explorercontract.GraphGroup(r.URL.Query().Get("group_by")),
		Group: r.URL.Query().Get("group"), Entity: r.URL.Query().Get("id"), Depth: depth,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeData(w, r, page, meta)
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	if !h.allowQuery(w, r, "severity", "limit") {
		return
	}
	limit, err := parseInt(r.URL.Query().Get("limit"), 100)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	report, meta, err := h.service.Health(explorercontract.HealthSeverity(r.URL.Query().Get("severity")), limit)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeData(w, r, report, meta)
}

func (h *handler) allowQuery(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	set := map[string]struct{}{}
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	for key := range r.URL.Query() {
		if _, ok := set[key]; !ok {
			h.writeError(w, r, usageError("unknown_query_parameter", "The request contains an unsupported query parameter."))
			return false
		}
	}
	return true
}

func values(r *http.Request, key string) []string {
	raw := r.URL.Query()[key]
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func (h *handler) writeData(w http.ResponseWriter, r *http.Request, data any, meta explorercontract.Meta) {
	payload, err := json.Marshal(data)
	if err != nil {
		h.writeError(w, r, &QueryError{Code: "encode_failed", Summary: "Explorer could not encode the response.", Status: 500})
		return
	}
	envelope := explorercontract.Envelope{
		SchemaVersion: explorercontract.SchemaVersion, OK: true, ProjectionMode: h.service.dataset.ProjectionMode,
		SourceDigest: h.service.dataset.SourceDigest, Data: payload, Meta: meta,
	}
	h.writeEnvelope(w, r, http.StatusOK, envelope)
}

func (h *handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	query := &QueryError{Code: "internal_error", Summary: "Explorer could not complete the request.", Status: http.StatusInternalServerError}
	var typed *QueryError
	if errors.As(err, &typed) {
		query = typed
	}
	if len(query.Summary) > 512 {
		query.Summary = "Explorer could not complete the request."
	}
	envelope := explorercontract.Envelope{
		SchemaVersion: explorercontract.SchemaVersion, OK: false, ProjectionMode: h.service.dataset.ProjectionMode,
		SourceDigest: h.service.dataset.SourceDigest, Error: &explorercontract.Error{Code: query.Code, Summary: query.Summary},
		Meta: explorercontract.Meta{Truncated: false},
	}
	h.writeEnvelope(w, r, query.Status, envelope)
}

func (h *handler) writeEnvelope(w http.ResponseWriter, r *http.Request, status int, envelope explorercontract.Envelope) {
	payload, err := json.Marshal(envelope)
	if err != nil || len(payload) > maxHTTPResponseBytes {
		status = http.StatusInternalServerError
		payload = []byte(`{"schema_version":1,"ok":false,"projection_mode":"local","source_digest":"","error":{"code":"response_too_large","summary":"Explorer could not return a bounded response."},"meta":{"truncated":false}}`)
	}
	etag := responseETag(h.service.dataset.SourceDigest, h.service.dataset.ProjectionMode, r.URL.Path, r.URL.Query().Encode(), payload)
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	if r.Header.Get("If-None-Match") == etag && status == http.StatusOK {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func responseETag(digest string, mode explorercontract.ProjectionMode, route, query string, payload []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(digest))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(mode))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(route))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(query))
	_, _ = hash.Write(payload)
	return `"` + hex.EncodeToString(hash.Sum(nil)) + `"`
}

func setSecurityHeaders(header http.Header) {
	header.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}

// AllowedHostsForListener returns exact Host values for one selected loopback listener.
func AllowedHostsForListener(address string) ([]string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	values := map[string]struct{}{net.JoinHostPort(host, port): {}}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		values[net.JoinHostPort("localhost", port)] = struct{}{}
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}
