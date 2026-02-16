package servertap

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"mcmm/internal/config"
	ilog "mcmm/internal/log"
)

func TestNewCommandBuilder_Build(t *testing.T) {
	cmd := NewCommandBuilder("mv").
		RawArg("import").
		Arg("world one").
		Arg("NORMAL").
		Build()

	want := "mv import 'world one' NORMAL"
	if cmd != want {
		t.Fatalf("unexpected command: got=%q want=%q", cmd, want)
	}
}

func TestParseHTTPResponse_Text(t *testing.T) {
	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader("internal error")),
	}
	parsed, err := ParseHTTPResponse(resp)
	if err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	if parsed.RawBody != "internal error" {
		t.Fatalf("unexpected raw body: %q", parsed.RawBody)
	}
}

func TestNewConnector_InvalidURL(t *testing.T) {
	_, err := NewConnector("://bad-url", 5*time.Second)
	if err == nil {
		t.Fatalf("expected invalid url error")
	}
}

func TestConnector_ExecuteMVList_WithConfig(t *testing.T) {
	if os.Getenv("RUN_SERVERTAP_E2E") != "1" {
		t.Skip("set RUN_SERVERTAP_E2E=1 to run real ServerTap integration test")
	}

	ilog.SetupLogger(ilog.LevelDebug)
	logger := ilog.Component("test")
	logger.Infof("loading config for servertap integration test")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	logger.Infof("using lobby_servertap_url=%s", cfg.LobbyServerTapURL)
	if strings.TrimSpace(cfg.ServerTapKey) == "" {
		t.Skip("servertap_key is empty in config/config.yml")
	}

	connector, err := NewConnectorWithAuth(cfg.LobbyServerTapURL, 10*time.Second, cfg.ServerTapAuthHeader, cfg.ServerTapKey)
	if err != nil {
		t.Fatalf("create connector failed: %v", err)
	}

	cmd := NewCommandBuilder("mv").RawArg("list").Build()
	logger.Infof("executing command: %s", cmd)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := connector.Execute(ctx, ExecuteRequest{Command: cmd})
	if err != nil {
		t.Fatalf("execute command failed: %v", err)
	}

	logger.Infof("status=%d", resp.StatusCode)
	logger.Infof("headers=%v", resp.Headers)
	logger.Infof("raw_body=%s", resp.RawBody)

	t.Logf("status=%d", resp.StatusCode)
	t.Logf("headers=%v", resp.Headers)
	t.Logf("raw_body=%s", resp.RawBody)
}
