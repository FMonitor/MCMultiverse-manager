package cmdreceiver

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"mcmm/internal/pgsql"
	"mcmm/internal/worker"
)

type WorldCommandRequest struct {
	Action      string `json:"action"`
	ActorUUID   string `json:"actor_uuid"`
	ActorName   string `json:"actor_name"`
	WorldAlias  string `json:"world_alias"`
	Target      string `json:"target_name"`
	RequestID   string `json:"request_id"`
	GameVersion string `json:"game_version"`
}

type WorldCommandResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type Service interface {
	HandleWorldCommand(ctx context.Context, req WorldCommandRequest) (int, WorldCommandResponse)
}

type HandlerI struct {
	service Service
}

func NewHandlerI(service Service) *HandlerI {
	return &HandlerI{service: service}
}

func (h *HandlerI) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/cmd/world", h.handleWorldCommand)
}

func (h *HandlerI) handleWorldCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, WorldCommandResponse{Status: "error", Message: "method not allowed"})
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "invalid form"})
		return
	}

	req := WorldCommandRequest{
		Action:      strings.TrimSpace(r.FormValue("action")),
		ActorUUID:   strings.TrimSpace(r.FormValue("actor_uuid")),
		ActorName:   strings.TrimSpace(r.FormValue("actor_name")),
		WorldAlias:  strings.TrimSpace(r.FormValue("world_alias")),
		Target:      strings.TrimSpace(r.FormValue("target_name")),
		RequestID:   strings.TrimSpace(r.FormValue("request_id")),
		GameVersion: strings.TrimSpace(r.FormValue("game_version")),
	}

	status, resp := h.service.HandleWorldCommand(r.Context(), req)
	writeJSON(w, status, resp)
}

type ServiceI struct {
	repos              pgsql.Repos
	worker             worker.Worker
	defaultGameVersion string
}

func NewServiceI(repos pgsql.Repos, w worker.Worker, defaultGameVersion string) *ServiceI {
	if defaultGameVersion == "" {
		defaultGameVersion = "1.21.1"
	}
	return &ServiceI{repos: repos, worker: w, defaultGameVersion: defaultGameVersion}
}

func (s *ServiceI) HandleWorldCommand(ctx context.Context, req WorldCommandRequest) (int, WorldCommandResponse) {
	req.Action = strings.TrimSpace(req.Action)
	req.ActorUUID = strings.TrimSpace(req.ActorUUID)
	req.ActorName = strings.TrimSpace(req.ActorName)
	req.WorldAlias = strings.TrimSpace(req.WorldAlias)
	req.Target = strings.TrimSpace(req.Target)
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.GameVersion = strings.TrimSpace(req.GameVersion)

	if req.Action == "" || req.ActorUUID == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "missing required fields"}
	}
	if req.RequestID == "" {
		req.RequestID = newUUIDLike()
	}

	actor, err := s.ensureActor(ctx, req.ActorUUID, req.ActorName)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "load actor failed"}
	}

	switch req.Action {
	case "create":
		return s.handleCreate(ctx, req, actor)
	case "delete":
		return s.handleDelete(ctx, req, actor)
	case "member_add":
		return s.handleMemberAdd(ctx, req, actor)
	case "member_remove":
		return s.handleMemberRemove(ctx, req, actor)
	default:
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "unsupported action"}
	}
}

func (s *ServiceI) handleCreate(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	createdReq, created, err := s.repos.UserRequest.CreateAcceptedIfNotExists(
		ctx,
		req.RequestID,
		"create_instance",
		sql.NullInt64{Int64: actor.ID, Valid: true},
		sql.NullInt64{},
	)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "create request failed"}
	}
	if !created {
		return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "duplicate request_id, using existing request"}
	}

	version := req.GameVersion
	if version == "" {
		version = s.defaultGameVersion
	}
	instanceID, err := s.repos.MapInstance.Create(ctx, pgsql.MapInstance{
		OwnerID:     actor.ID,
		SourceType:  "upload",
		GameVersion: version,
		Status:      string(worker.StatusPendingApproval),
	})
	if err != nil {
		_ = s.repos.UserRequest.MarkRequestResult(ctx, req.RequestID, "failed", json.RawMessage(`{"step":"create_instance_row"}`), sql.NullString{String: "db_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "create instance failed"}
	}
	_, _ = s.repos.InstanceMember.Create(ctx, pgsql.InstanceMember{
		InstanceID: instanceID,
		UserID:     actor.ID,
		Role:       "owner",
	})

	createdReq.TargetInstanceID = sql.NullInt64{Int64: instanceID, Valid: true}
	createdReq.Status = "processing"
	_ = s.repos.UserRequest.Update(ctx, createdReq)

	if err := s.worker.StartEmpty(ctx, instanceID, version); err != nil {
		_ = s.repos.UserRequest.MarkRequestResult(ctx, req.RequestID, "failed", json.RawMessage(`{"step":"start_empty"}`), sql.NullString{String: "worker_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "worker start failed"}
	}
	payload := fmt.Sprintf(`{"instance_id":%d,"game_version":"%s"}`, instanceID, version)
	_ = s.repos.UserRequest.MarkRequestResult(ctx, req.RequestID, "succeeded", json.RawMessage(payload), sql.NullString{}, sql.NullString{})
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("create accepted, instance_id=%d", instanceID)}
}

func (s *ServiceI) handleDelete(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	instanceID, err := parseInstanceID(req.WorldAlias)
	if err != nil {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "world_alias must be instance id"}
	}
	inst, err := s.repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	if !canManage(actor, inst.OwnerID) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "permission denied"}
	}

	_, _, err = s.repos.UserRequest.CreateAcceptedIfNotExists(
		ctx,
		req.RequestID,
		"delete_instance",
		sql.NullInt64{Int64: actor.ID, Valid: true},
		sql.NullInt64{Int64: instanceID, Valid: true},
	)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "delete request failed"}
	}
	if err := s.worker.StopAndArchive(ctx, instanceID); err != nil {
		_ = s.repos.UserRequest.MarkRequestResult(ctx, req.RequestID, "failed", json.RawMessage(`{"step":"stop_archive"}`), sql.NullString{String: "worker_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "archive failed"}
	}
	_ = s.repos.UserRequest.MarkRequestResult(ctx, req.RequestID, "succeeded", json.RawMessage(fmt.Sprintf(`{"instance_id":%d}`, instanceID)), sql.NullString{}, sql.NullString{})
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "delete accepted"}
}

func (s *ServiceI) handleMemberAdd(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	instanceID, err := parseInstanceID(req.WorldAlias)
	if err != nil {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "world_alias must be instance id"}
	}
	inst, err := s.repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	if !canManage(actor, inst.OwnerID) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "permission denied"}
	}
	target, err := s.repos.User.ReadByName(ctx, req.Target)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "target user not found (must join once)"}
	}
	if _, err := s.repos.InstanceMember.Create(ctx, pgsql.InstanceMember{
		InstanceID: instanceID,
		UserID:     target.ID,
		Role:       "member",
	}); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "already a member"}
		}
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "add member failed"}
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "member added"}
}

func (s *ServiceI) handleMemberRemove(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	instanceID, err := parseInstanceID(req.WorldAlias)
	if err != nil {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "world_alias must be instance id"}
	}
	inst, err := s.repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	if !canManage(actor, inst.OwnerID) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "permission denied"}
	}
	target, err := s.repos.User.ReadByName(ctx, req.Target)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "target user not found"}
	}
	if err := s.repos.InstanceMember.DeleteByInstanceAndUser(ctx, instanceID, target.ID); err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "remove member failed"}
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "member removed"}
}

func (s *ServiceI) ensureActor(ctx context.Context, actorUUID, actorName string) (pgsql.User, error) {
	u, err := s.repos.User.ReadByUUID(ctx, actorUUID)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return pgsql.User{}, err
	}
	if strings.TrimSpace(actorName) == "" {
		actorName = "unknown"
	}
	id, err := s.repos.User.Create(ctx, pgsql.User{
		MCUUID:     actorUUID,
		MCName:     actorName,
		ServerRole: "user",
	})
	if err != nil {
		return pgsql.User{}, err
	}
	return s.repos.User.Read(ctx, id)
}

func canManage(actor pgsql.User, ownerID int64) bool {
	return actor.ServerRole == "admin" || actor.ID == ownerID
}

func parseInstanceID(alias string) (int64, error) {
	s := strings.TrimSpace(alias)
	s = strings.TrimPrefix(s, "inst-")
	return strconv.ParseInt(s, 10, 64)
}

func newUUIDLike() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
