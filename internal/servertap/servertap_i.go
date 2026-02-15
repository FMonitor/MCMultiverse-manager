package servertap

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	ilog "mcmm/internal/log"
)

const (
	DefaultExecutePath = "/v1/server/exec"
)

type Connector struct {
	baseURL    *url.URL
	client     *http.Client
	authHeader string
	authKey    string
}

type ExecuteRequest struct {
	Command string
	Path    string
}

type ExecutePayload struct {
	Command string `json:"command"`
}

type ParsedResponse struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	RawBody    string              `json:"raw_body"`
}

type CommandBuilder struct {
	tokens []string
}

func NewConnector(baseURL string, timeout time.Duration) (*Connector, error) {
	return NewConnectorWithAuth(baseURL, timeout, "key", "")
}

func NewConnectorWithAuth(baseURL string, timeout time.Duration, authHeader string, authKey string) (*Connector, error) {
	normalized := strings.TrimSpace(baseURL)
	if normalized == "" {
		return nil, fmt.Errorf("servertap base url is required")
	}

	u, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("invalid servertap url: %w", err)
	}

	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid servertap url, need scheme and host: %s", normalized)
	}

	clientTimeout := timeout
	if clientTimeout < 0 {
		clientTimeout = 10 * time.Second
	}

	header := strings.TrimSpace(authHeader)
	if header == "" {
		header = "key"
	}

	return &Connector{
		baseURL: u,
		client: &http.Client{
			Timeout: clientTimeout,
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
		authHeader: header,
		authKey:    strings.TrimSpace(authKey),
	}, nil
}

func NewCommandBuilder(base string) *CommandBuilder {
	base = strings.TrimSpace(base)
	if base == "" {
		return &CommandBuilder{tokens: []string{}}
	}
	return &CommandBuilder{tokens: []string{base}}
}

func (b *CommandBuilder) Arg(value string) *CommandBuilder {
	b.tokens = append(b.tokens, quoteIfNeeded(value))
	return b
}

func (b *CommandBuilder) RawArg(value string) *CommandBuilder {
	b.tokens = append(b.tokens, strings.TrimSpace(value))
	return b
}

func (b *CommandBuilder) Build() string {
	return strings.TrimSpace(strings.Join(b.tokens, " "))
}

func quoteIfNeeded(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "''"
	}
	if !strings.ContainsAny(trimmed, " \t\"'") {
		return trimmed
	}
	escaped := strings.ReplaceAll(trimmed, `'`, `'\''`)
	return "'" + escaped + "'"
}

func (c *Connector) Execute(ctx context.Context, req ExecuteRequest) (ParsedResponse, error) {
	logger := ilog.Component("servertap")
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return ParsedResponse{}, fmt.Errorf("command is required")
	}

	path := strings.TrimSpace(req.Path)
	if path == "" {
		path = DefaultExecutePath
	}

	endpoint := c.baseURL.ResolveReference(&url.URL{Path: path})
	payload := ExecutePayload{
		Command: command,
	}

	form := url.Values{}
	form.Set("command", payload.Command)

	logger.Infof("sending command to servertap: %s", command)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return ParsedResponse{}, fmt.Errorf("build execute request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.authKey != "" {
		httpReq.Header.Set(c.authHeader, c.authKey)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return ParsedResponse{}, fmt.Errorf("execute request failed: %w", err)
	}
	defer resp.Body.Close()

	parsed, err := ParseHTTPResponse(resp)
	if err != nil {
		return ParsedResponse{}, err
	}
	bodyPreview := strings.TrimSpace(parsed.RawBody)
	if len(bodyPreview) > 240 {
		bodyPreview = bodyPreview[:240] + "..."
	}
	logger.Infof("servertap response status=%d body_bytes=%d body=%q", parsed.StatusCode, len(parsed.RawBody), bodyPreview)
	return parsed, nil
}

func ParseHTTPResponse(resp *http.Response) (ParsedResponse, error) {
	if resp == nil {
		return ParsedResponse{}, fmt.Errorf("nil http response")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ParsedResponse{}, fmt.Errorf("read response body failed: %w", err)
	}

	out := ParsedResponse{
		StatusCode: resp.StatusCode,
		Headers:    cloneHeader(resp.Header),
		RawBody:    string(body),
	}

	// Parsing is intentionally deferred to c-layer/domain-specific logic.
	return out, nil
}

func cloneHeader(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}
