package pgsql

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"mcmm/internal/config"
	ilog "mcmm/internal/log"
)

func TestRepos_CreateMockData(t *testing.T) {
	ctx := context.Background()

	ilog.SetupLogger(ilog.LevelDebug)
	logger := ilog.Logger.With("component", "repo_c_test")

	cfg, err := config.Load()
	if err != nil {
		t.Skipf("skip integration test without local config: %v", err)
	}
	logger.Infof("config loaded")

	dsn := cfg.DBURL
	if override := os.Getenv("TEST_DATABASE_URL"); override != "" {
		dsn = override
		logger.Infof("using TEST_DATABASE_URL override")
	} else {
		t.Skip("skip integration test unless TEST_DATABASE_URL is set")
	}

	connector := NewConnector(dsn)
	if err := connector.Connect(ctx); err != nil {
		t.Fatalf("connect db failed: %v", err)
	}
	defer connector.Close()
	logger.Infof("database connected")

	repos := NewRepos(connector)

	userUUID := newUUIDLike()
	userID, err := repos.User.Create(ctx, User{
		MCUUID:     userUUID,
		MCName:     "repo_test_user",
		ServerRole: "user",
	})
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	templateID, err := repos.MapTemplate.Create(ctx, MapTemplate{
		Tag:         "repo-test-" + shortHex(4),
		DisplayName: "Repo Test Template",
		GameVersion: "1.21",
		BlobPath:    "/data/templates/repo-test.tar.zst",
	})
	if err != nil {
		t.Fatalf("create map template failed: %v", err)
	}

	serverImageID := "s-" + shortHex(3)
	err = repos.ServerImage.Create(ctx, ServerImage{
		ID:          serverImageID,
		Name:        "Repo Test Image",
		GameVersion: "1.21.1",
	})
	if err != nil {
		t.Fatalf("create server image failed: %v", err)
	}

	instanceID, err := repos.MapInstance.Create(ctx, MapInstance{
		Alias:       "repo-inst-" + shortHex(4),
		OwnerID:     userID,
		TemplateID:  sql.NullInt64{Int64: templateID, Valid: true},
		SourceType:  "template",
		GameVersion: "1.21.1",
		AccessMode:  "privacy",
		Status:      "Waiting",
	})
	if err != nil {
		t.Fatalf("create map instance failed: %v", err)
	}

	memberID, err := repos.InstanceMember.Create(ctx, InstanceMember{
		InstanceID: instanceID,
		UserID:     userID,
		Role:       "owner",
	})
	if err != nil {
		t.Fatalf("create instance member failed: %v", err)
	}

	requestID := newUUIDLike()
	req, created, err := repos.UserRequest.CreateAcceptedIfNotExists(
		ctx,
		requestID,
		"create_instance",
		sql.NullInt64{Int64: userID, Valid: true},
		sql.NullInt64{Int64: instanceID, Valid: true},
	)
	if err != nil {
		t.Fatalf("create accepted request failed: %v", err)
	}
	if !created {
		t.Fatalf("expected new user_request row, got existing one")
	}

	err = repos.UserRequest.MarkRequestResult(
		ctx,
		requestID,
		"succeeded",
		json.RawMessage(`{"instance_id":1}`),
		sql.NullString{},
		sql.NullString{},
	)
	if err != nil {
		t.Fatalf("mark request result failed: %v", err)
	}

	_, err = repos.User.Read(ctx, userID)
	if err != nil {
		t.Fatalf("read user failed: %v", err)
	}
	_, err = repos.MapTemplate.Read(ctx, templateID)
	if err != nil {
		t.Fatalf("read map template failed: %v", err)
	}
	_, err = repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		t.Fatalf("read map instance failed: %v", err)
	}
	_, err = repos.InstanceMember.Read(ctx, memberID)
	if err != nil {
		t.Fatalf("read instance member failed: %v", err)
	}
	_, err = repos.ServerImage.Read(ctx, serverImageID)
	if err != nil {
		t.Fatalf("read server image failed: %v", err)
	}
	images, err := repos.ServerImage.List(ctx)
	if err != nil {
		t.Fatalf("list server images failed: %v", err)
	}
	if len(images) == 0 {
		t.Fatalf("expected at least one server image")
	}
	_, err = repos.UserRequest.Read(ctx, req.ID)
	if err != nil {
		t.Fatalf("read user_request failed: %v", err)
	}

	t.Logf("mock data inserted: user=%d template=%d instance=%d member=%d request=%d", userID, templateID, instanceID, memberID, req.ID)
	t.Logf("check your DB now; rows are kept intentionally")
	logger.Infof("mock data inserted successfully")
}

func newUUIDLike() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// RFC4122 version/variant bits
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func shortHex(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
