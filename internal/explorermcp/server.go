// Package explorermcp exposes bounded read-only Explorer tools over MCP stdio.
package explorermcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/nstranquist/nicos-catalog/internal/explorerapi"
	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

const (
	maxInputBytes  = 1 << 20
	maxResultBytes = 64 << 10
	protocol       = "2025-06-18"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// Server exposes one immutable Explorer projection through bounded MCP tools.
type Server struct {
	service        *explorerapi.Service
	productVersion string
}

// New returns a read-only MCP server for service.
func New(service *explorerapi.Service, productVersion string) (*Server, error) {
	if service == nil {
		return nil, fmt.Errorf("explorer service is required")
	}
	return &Server{service: service, productVersion: productVersion}, nil
}

// Serve processes one JSON-RPC message per line until EOF or cancellation.
func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), maxInputBytes)
	writer := bufio.NewWriter(output)
	defer func() { _ = writer.Flush() }()
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var message request
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&message); err != nil {
			if err := writeResponse(writer, response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "Invalid JSON-RPC message."}}); err != nil {
				return err
			}
			continue
		}
		if message.JSONRPC != "2.0" || strings.TrimSpace(message.Method) == "" {
			if err := writeResponse(writer, response{JSONRPC: "2.0", ID: message.ID, Error: &rpcError{Code: -32600, Message: "Invalid JSON-RPC request."}}); err != nil {
				return err
			}
			continue
		}
		if len(message.ID) == 0 && strings.HasPrefix(message.Method, "notifications/") {
			continue
		}
		result := s.handle(ctx, message)
		if err := writeResponse(writer, result); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("MCP input exceeded its bound or could not be read")
	}
	return ctx.Err()
}

func (s *Server) handle(ctx context.Context, message request) response {
	base := response{JSONRPC: "2.0", ID: message.ID}
	if err := ctx.Err(); err != nil {
		base.Error = &rpcError{Code: -32800, Message: "Request canceled."}
		return base
	}
	switch message.Method {
	case "initialize":
		base.Result = map[string]any{
			"protocolVersion": protocol,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "nicos-catalog", "version": s.productVersion},
		}
	case "ping":
		base.Result = map[string]any{}
	case "tools/list":
		base.Result = map[string]any{"tools": tools()}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := decodeStrict(message.Params, &params); err != nil {
			base.Error = &rpcError{Code: -32602, Message: "Invalid tool arguments."}
			return base
		}
		result, err := s.call(params.Name, params.Arguments)
		if err != nil {
			base.Result = toolError(err)
			return base
		}
		base.Result = result
	default:
		base.Error = &rpcError{Code: -32601, Message: "Method not found."}
	}
	return base
}

func (s *Server) call(name string, raw json.RawMessage) (toolResult, error) {
	var value any
	switch name {
	case "catalog_search":
		var args struct {
			Query   string   `json:"query"`
			Kind    []string `json:"kind,omitempty"`
			Status  []string `json:"status,omitempty"`
			Surface []string `json:"surface,omitempty"`
			Tag     []string `json:"tag,omitempty"`
			Limit   int      `json:"limit,omitempty"`
		}
		if err := decodeStrict(raw, &args); err != nil {
			return toolResult{}, err
		}
		if args.Limit == 0 {
			args.Limit = 20
		}
		if args.Limit > 20 {
			return toolResult{}, fmt.Errorf("search is limited to 20 results")
		}
		page, meta, err := s.service.Search(explorerapi.SearchOptions{Query: args.Query, Kinds: args.Kind, Statuses: args.Status, Surfaces: args.Surface, Tags: args.Tag, Limit: args.Limit})
		if err != nil {
			return toolResult{}, err
		}
		value = map[string]any{"data": page, "meta": meta}
	case "catalog_get":
		var args struct {
			ID string `json:"id"`
		}
		if err := decodeStrict(raw, &args); err != nil {
			return toolResult{}, err
		}
		detail, meta, err := s.service.EntityDetail(args.ID, 50)
		if err != nil {
			return toolResult{}, err
		}
		value = map[string]any{"data": detail, "meta": meta}
	case "catalog_graph":
		var args struct {
			Mode    explorercontract.GraphMode  `json:"mode,omitempty"`
			GroupBy explorercontract.GraphGroup `json:"group_by,omitempty"`
			Group   string                      `json:"group,omitempty"`
			ID      string                      `json:"id,omitempty"`
			Depth   int                         `json:"depth,omitempty"`
		}
		if err := decodeStrict(raw, &args); err != nil {
			return toolResult{}, err
		}
		page, meta, err := s.service.Graph(explorerapi.GraphOptions{Mode: args.Mode, GroupBy: args.GroupBy, Group: args.Group, Entity: args.ID, Depth: args.Depth, MaxNodes: 200, MaxEdges: 500})
		if err != nil {
			return toolResult{}, err
		}
		value = map[string]any{"data": page, "meta": meta}
	case "catalog_health":
		var args struct {
			Severity explorercontract.HealthSeverity `json:"severity,omitempty"`
			Limit    int                             `json:"limit,omitempty"`
		}
		if err := decodeStrict(raw, &args); err != nil {
			return toolResult{}, err
		}
		if args.Limit == 0 {
			args.Limit = 50
		}
		if args.Limit > 50 {
			return toolResult{}, fmt.Errorf("health is limited to 50 findings")
		}
		report, meta, err := s.service.Health(args.Severity, args.Limit)
		if err != nil {
			return toolResult{}, err
		}
		value = map[string]any{"data": report, "meta": meta}
	default:
		return toolResult{}, fmt.Errorf("unknown read-only catalog tool")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return toolResult{}, fmt.Errorf("tool result could not be encoded")
	}
	if len(payload) > maxResultBytes {
		return toolResult{}, fmt.Errorf("tool result exceeds the 64 KiB limit")
	}
	return toolResult{Content: []toolContent{{Type: "text", Text: string(payload)}}}, nil
}

func tools() []map[string]any {
	return []map[string]any{
		{"name": "catalog_search", "description": "Search projected catalog fields. Returns at most 20 results.", "inputSchema": objectSchema(map[string]any{"query": stringSchema(), "kind": stringArray(), "status": stringArray(), "surface": stringArray(), "tag": stringArray(), "limit": integerSchema(1, 20)}, "query")},
		{"name": "catalog_get", "description": "Get one projected entity and at most 50 direct relationships.", "inputSchema": objectSchema(map[string]any{"id": stringSchema()}, "id")},
		{"name": "catalog_graph", "description": "Read an aggregate, region, or depth-two neighborhood graph.", "inputSchema": objectSchema(map[string]any{"mode": enumSchema("aggregate", "region", "neighborhood"), "group_by": enumSchema("kind", "surface"), "group": stringSchema(), "id": stringSchema(), "depth": integerSchema(1, 2)})},
		{"name": "catalog_health", "description": "Read at most 50 redacted health findings.", "inputSchema": objectSchema(map[string]any{"severity": enumSchema("error", "warning", "info"), "limit": integerSchema(1, 50)})},
	}
}

func toolError(err error) toolResult {
	summary := "The read-only catalog tool could not complete the request."
	var queryErr *explorerapi.QueryError
	if errors.As(err, &queryErr) {
		summary = queryErr.Summary
	} else if len(err.Error()) <= 160 {
		summary = err.Error()
	}
	return toolResult{Content: []toolContent{{Type: "text", Text: summary}}, IsError: true}
}
func writeResponse(writer *bufio.Writer, value response) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload) > maxResultBytes+4096 {
		return fmt.Errorf("MCP response exceeds its bound")
	}
	if _, err := writer.Write(append(payload, '\n')); err != nil {
		return err
	}
	return writer.Flush()
}
func decodeStrict(raw []byte, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("extra JSON value")
	}
	return nil
}
func objectSchema(properties map[string]any, required ...string) map[string]any {
	out := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}
func stringSchema() map[string]any { return map[string]any{"type": "string", "maxLength": 256} }
func stringArray() map[string]any {
	return map[string]any{"type": "array", "items": stringSchema(), "maxItems": 20}
}
func integerSchema(minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "minimum": minimum, "maximum": maximum}
}
func enumSchema(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}
