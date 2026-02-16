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
	"time"

	"mcmm/internal/log"
	"mcmm/internal/pgsql"
	"mcmm/internal/servertap"
	"mcmm/internal/worker"
)

type WorldCommandRequest struct {
	Action       string `json:"action"`
	ActorUUID    string `json:"actor_uuid"`
	ActorName    string `json:"actor_name"`
	WorldAlias   string `json:"world_alias"`
	Target       string `json:"target_name"`
	RequestID    string `json:"request_id"`
	GameVersion  string `json:"game_version"`
	TemplateName string `json:"template_name"`
	Reason       string `json:"reason"`
	AccessMode   string `json:"access_mode"`
}

type WorldCommandResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type Service interface {
	HandleWorldCommand(ctx context.Context, req WorldCommandRequest) (int, WorldCommandResponse)
	HandlePlayerJoin(ctx context.Context, actorUUID string, actorName string) (int, WorldCommandResponse)
}

type HandlerI struct {
	service Service
}

func NewHandlerI(service Service) *HandlerI {
	return &HandlerI{service: service}
}

func (h *HandlerI) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/cmd/world", h.handleWorldCommand)
	mux.HandleFunc("/v1/cmd/player/join", h.handlePlayerJoin)
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
		Action:       strings.TrimSpace(r.FormValue("action")),
		ActorUUID:    strings.TrimSpace(r.FormValue("actor_uuid")),
		ActorName:    strings.TrimSpace(r.FormValue("actor_name")),
		WorldAlias:   strings.TrimSpace(r.FormValue("world_alias")),
		Target:       strings.TrimSpace(r.FormValue("target_name")),
		RequestID:    strings.TrimSpace(r.FormValue("request_id")),
		GameVersion:  strings.TrimSpace(r.FormValue("game_version")),
		TemplateName: strings.TrimSpace(r.FormValue("template_name")),
		Reason:       strings.TrimSpace(r.FormValue("reason")),
		AccessMode:   strings.TrimSpace(r.FormValue("access_mode")),
	}

	status, resp := h.service.HandleWorldCommand(r.Context(), req)
	writeJSON(w, status, resp)
}

func (h *HandlerI) handlePlayerJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, WorldCommandResponse{Status: "error", Message: "method not allowed"})
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "invalid form"})
		return
	}
	actorUUID := strings.TrimSpace(r.FormValue("actor_uuid"))
	actorName := strings.TrimSpace(r.FormValue("actor_name"))
	status, resp := h.service.HandlePlayerJoin(r.Context(), actorUUID, actorName)
	writeJSON(w, status, resp)
}

type ServiceI struct {
	repos              pgsql.Repos
	worker             worker.Worker
	defaultGameVersion string
	lobbyTapURL        string
	serverTapKey       string
	serverTapAuthName  string
	logger             interface {
		Infof(string, ...any)
		Warnf(string, ...any)
		Errorf(string, ...any)
	}
}

func NewServiceI(repos pgsql.Repos, w worker.Worker, defaultGameVersion string, lobbyTapURL string, serverTapAuthName string, serverTapKey string) *ServiceI {
	if defaultGameVersion == "" {
		defaultGameVersion = "1.21.1"
	}
	return &ServiceI{
		repos:              repos,
		worker:             w,
		defaultGameVersion: defaultGameVersion,
		lobbyTapURL:        strings.TrimSpace(lobbyTapURL),
		serverTapAuthName:  strings.TrimSpace(serverTapAuthName),
		serverTapKey:       strings.TrimSpace(serverTapKey),
		logger:             log.Component("cmdreceiver"),
	}
}

func (s *ServiceI) HandleWorldCommand(ctx context.Context, req WorldCommandRequest) (int, WorldCommandResponse) {
	req.Action = strings.TrimSpace(req.Action)
	req.ActorUUID = strings.TrimSpace(req.ActorUUID)
	req.ActorName = strings.TrimSpace(req.ActorName)
	req.WorldAlias = strings.TrimSpace(req.WorldAlias)
	req.Target = strings.TrimSpace(req.Target)
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.GameVersion = strings.TrimSpace(req.GameVersion)
	req.TemplateName = strings.TrimSpace(req.TemplateName)
	req.Reason = strings.TrimSpace(req.Reason)
	req.AccessMode = strings.TrimSpace(strings.ToLower(req.AccessMode))

	if req.Action == "" || req.ActorUUID == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "missing required fields"}
	}
	if req.RequestID == "" {
		req.RequestID = newUUIDLike()
	}

	actor, err := s.ensureActor(ctx, req.ActorUUID, req.ActorName)
	if err != nil {
		s.logger.Errorf("load actor failed action=%s actor=%s uuid=%s err=%v", req.Action, req.ActorName, req.ActorUUID, err)
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "load actor failed"}
	}
	s.logger.Infof(
		"world_cmd actor=%s uuid=%s role=%s action=%s req_id=%s world=%s target=%s template=%s access=%s",
		actor.MCName, actor.MCUUID, actor.ServerRole, req.Action, req.RequestID, req.WorldAlias, req.Target, req.TemplateName, req.AccessMode,
	)
	if isOpOnlyAction(req.Action) && !isAdmin(actor) {
		s.logger.Warnf("world_cmd forbidden actor=%s uuid=%s role=%s action=%s", actor.MCName, actor.MCUUID, actor.ServerRole, req.Action)
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}

	switch req.Action {
	case "create", "request_create":
		return s.handleRequestCreate(ctx, req, actor)
	case "request_list":
		return s.handleRequestList(ctx, actor)
	case "request_approve":
		return s.handleRequestApprove(ctx, req, actor)
	case "request_reject":
		return s.handleRequestReject(ctx, req, actor)
	case "request_cancel":
		return s.handleRequestCancel(ctx, req, actor)
	case "world_list":
		return s.handleWorldList(ctx, actor)
	case "world_info":
		return s.handleWorldInfo(ctx, req, actor)
	case "world_join":
		return s.handleWorldJoin(ctx, req, actor)
	case "world_set_access":
		return s.handleWorldSetAccess(ctx, req, actor)
	case "world_remove", "delete":
		return s.handleDelete(ctx, req, actor)
	case "member_add":
		return s.handleMemberAdd(ctx, req, actor)
	case "member_remove":
		return s.handleMemberRemove(ctx, req, actor)
	case "instance_list":
		return s.handleInstanceList(ctx, actor)
	case "template_list":
		return s.handleTemplateList(ctx)
	case "create_legacy":
		return s.handleCreate(ctx, req, actor)
	default:
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "unsupported action"}
	}
}

func (s *ServiceI) HandlePlayerJoin(ctx context.Context, actorUUID string, actorName string) (int, WorldCommandResponse) {
	actorUUID = strings.TrimSpace(actorUUID)
	actorName = strings.TrimSpace(actorName)
	if actorUUID == "" || actorName == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "missing actor_uuid or actor_name"}
	}
	s.logger.Infof("player_join actor=%s uuid=%s", actorName, actorUUID)
	user, err := s.ensureActor(ctx, actorUUID, actorName)
	if err != nil {
		s.logger.Errorf("player_join upsert failed actor=%s uuid=%s err=%v", actorName, actorUUID, err)
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "upsert user failed"}
	}
	s.logger.Infof("player_join synced actor=%s uuid=%s user_id=%d role=%s", actorName, actorUUID, user.ID, user.ServerRole)
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("player synced id=%d", user.ID)}
}

func (s *ServiceI) handleRequestCreate(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if req.WorldAlias == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "world_alias is required"}
	}
	finalAlias := buildOwnedAlias(actor.MCName, req.WorldAlias)
	if req.RequestID == "" {
		req.RequestID = newUUIDLike()
	}

	if _, err := s.repos.MapInstance.ReadByAlias(ctx, finalAlias); err == nil {
		return http.StatusConflict, WorldCommandResponse{Status: "error", Message: "world_alias already exists"}
	}

	var (
		template   pgsql.MapTemplate
		templateID sql.NullInt64
		err        error
	)
	if req.TemplateName != "" {
		template, err = s.resolveTemplate(ctx, req.TemplateName)
		if err != nil {
			return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "template not found"}
		}
		templateID = sql.NullInt64{Int64: template.ID, Valid: true}
	}

	ur, err := s.repos.UserRequest.ReadByRequestID(ctx, req.RequestID)
	if err == nil {
		return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("duplicate request_id, current status=%s", ur.Status)}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "read request failed"}
	}

	requestNo, err := s.repos.UserRequest.Create(ctx, pgsql.UserRequest{
		RequestID:      req.RequestID,
		RequestType:    "world_create",
		ActorUserID:    actor.ID,
		TemplateID:     templateID,
		RequestedAlias: sql.NullString{String: finalAlias, Valid: true},
		Status:         "pending",
		ResponsePayload: json.RawMessage(
			fmt.Sprintf(`{"template":"%s","world_alias":"%s"}`, req.TemplateName, finalAlias),
		),
	})
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "create request failed"}
	}
	_ = s.notifyLobbyAdminsRequestCreated(ctx, actor.MCName, finalAlias, req.TemplateName, requestNo, req.RequestID)

	return http.StatusOK, WorldCommandResponse{
		Status:  "accepted",
		Message: fmt.Sprintf("request created, request_no=%d, request_id=%s, world_alias=%s", requestNo, req.RequestID, finalAlias),
	}
}

func (s *ServiceI) handleRequestList(ctx context.Context, actor pgsql.User) (int, WorldCommandResponse) {
	const limit = 20
	var (
		rows []pgsql.UserRequest
		err  error
	)
	if isAdmin(actor) {
		rows, err = s.repos.UserRequest.ListPending(ctx, limit)
	} else {
		rows, err = s.repos.UserRequest.ListByActor(ctx, actor.ID, limit)
	}
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "list requests failed"}
	}
	if len(rows) == 0 {
		return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "no requests"}
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		actorName := fmt.Sprintf("uid:%d", r.ActorUserID)
		if u, uErr := s.repos.User.Read(ctx, r.ActorUserID); uErr == nil {
			actorName = u.MCName
		}
		worldAlias := "-"
		if r.RequestedAlias.Valid {
			worldAlias = r.RequestedAlias.String
		}
		templateName := "empty"
		if r.TemplateID.Valid {
			if t, tErr := s.repos.MapTemplate.Read(ctx, r.TemplateID.Int64); tErr == nil {
				templateName = fmt.Sprintf("#%d:%s", t.ID, t.Tag)
			}
		}
		out = append(out, fmt.Sprintf("#%d:%s player=%s world=%s template=%s", r.ID, r.Status, actorName, worldAlias, templateName))
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: strings.Join(out, ", ")}
}

func (s *ServiceI) handleRequestApprove(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if !isAdmin(actor) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}
	if req.RequestID == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "request_id_or_no is required"}
	}
	ur, err := s.resolveUserRequest(ctx, req.RequestID)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "request not found"}
	}
	if ur.Status != "pending" {
		return http.StatusConflict, WorldCommandResponse{Status: "error", Message: fmt.Sprintf("request status is %s", ur.Status)}
	}
	if ur.RequestType != "world_create" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "request_type is not world_create"}
	}
	if !ur.RequestedAlias.Valid {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "request payload incomplete"}
	}

	ur.Status = "processing"
	ur.ReviewedByUserID = sql.NullInt64{Int64: actor.ID, Valid: true}
	ur.TargetInstanceID = sql.NullInt64{}
	if err := s.repos.UserRequest.Update(ctx, ur); err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "update request failed"}
	}

	go s.processApproveAsync(ur)
	return http.StatusAccepted, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("request #%d accepted, processing started", ur.ID)}
}

func (s *ServiceI) processApproveAsync(ur pgsql.UserRequest) {
	ctx := context.Background()

	instance := pgsql.MapInstance{
		Alias:       ur.RequestedAlias.String,
		OwnerID:     ur.ActorUserID,
		TemplateID:  ur.TemplateID,
		SourceType:  "empty",
		GameVersion: s.defaultGameVersion,
		AccessMode:  "privacy",
		Status:      string(worker.StatusWaiting),
	}

	var (
		template pgsql.MapTemplate
		err      error
	)
	if ur.TemplateID.Valid {
		template, err = s.repos.MapTemplate.Read(ctx, ur.TemplateID.Int64)
		if err != nil {
			_ = s.repos.UserRequest.MarkRequestResult(ctx, ur.RequestID, "failed", json.RawMessage(`{"step":"load_template"}`), sql.NullString{String: "db_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
			s.notifyApproveResult(ctx, ur, false, 0, "template not found")
			return
		}
		instance.SourceType = "template"
		instance.GameVersion = template.GameVersion
	}

	instanceID, err := s.repos.MapInstance.Create(ctx, instance)
	if err != nil {
		_ = s.repos.UserRequest.MarkRequestResult(ctx, ur.RequestID, "failed", json.RawMessage(`{"step":"create_instance_row"}`), sql.NullString{String: "db_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
		s.notifyApproveResult(ctx, ur, false, 0, "create instance failed")
		return
	}
	_, _ = s.repos.InstanceMember.Create(ctx, pgsql.InstanceMember{
		InstanceID: instanceID,
		UserID:     ur.ActorUserID,
		Role:       "owner",
	})

	if ur.TemplateID.Valid {
		if err := s.worker.StartFromTemplate(ctx, instanceID, template); err != nil {
			_ = s.repos.UserRequest.MarkRequestResult(ctx, ur.RequestID, "failed", json.RawMessage(`{"step":"start_template"}`), sql.NullString{String: "worker_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
			s.notifyApproveResult(ctx, ur, false, instanceID, "start template failed")
			return
		}
	} else {
		if err := s.worker.StartEmpty(ctx, instanceID, instance.GameVersion); err != nil {
			_ = s.repos.UserRequest.MarkRequestResult(ctx, ur.RequestID, "failed", json.RawMessage(`{"step":"start_empty"}`), sql.NullString{String: "worker_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
			s.notifyApproveResult(ctx, ur, false, instanceID, "start empty failed")
			return
		}
	}
	_ = s.repos.UserRequest.MarkRequestResult(ctx, ur.RequestID, "succeeded", json.RawMessage(fmt.Sprintf(`{"instance_id":%d}`, instanceID)), sql.NullString{}, sql.NullString{})
	s.notifyApproveResult(ctx, ur, true, instanceID, "")
}

func (s *ServiceI) handleRequestReject(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if !isAdmin(actor) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}
	if req.RequestID == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "request_id_or_no is required"}
	}
	ur, err := s.resolveUserRequest(ctx, req.RequestID)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "request not found"}
	}
	if ur.Status != "pending" {
		return http.StatusConflict, WorldCommandResponse{Status: "error", Message: fmt.Sprintf("request status is %s", ur.Status)}
	}
	ur.Status = "rejected"
	ur.ReviewedByUserID = sql.NullInt64{Int64: actor.ID, Valid: true}
	if req.Reason != "" {
		ur.ReviewNote = sql.NullString{String: req.Reason, Valid: true}
	}
	if err := s.repos.UserRequest.Update(ctx, ur); err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "update request failed"}
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "request rejected"}
}

func (s *ServiceI) handleRequestCancel(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if req.RequestID == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "request_id_or_no is required"}
	}
	ur, err := s.resolveUserRequest(ctx, req.RequestID)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "request not found"}
	}
	if !isAdmin(actor) && ur.ActorUserID != actor.ID {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "permission denied"}
	}
	if ur.Status != "pending" {
		return http.StatusConflict, WorldCommandResponse{Status: "error", Message: fmt.Sprintf("request status is %s", ur.Status)}
	}
	ur.Status = "canceled"
	if req.Reason != "" {
		ur.ReviewNote = sql.NullString{String: req.Reason, Valid: true}
	}
	if err := s.repos.UserRequest.Update(ctx, ur); err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "update request failed"}
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "request canceled"}
}

func (s *ServiceI) handleTemplateList(ctx context.Context) (int, WorldCommandResponse) {
	templates, err := s.repos.MapTemplate.List(ctx)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "list templates failed"}
	}
	if len(templates) == 0 {
		return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "no templates found"}
	}
	limit := len(templates)
	if limit > 20 {
		limit = 20
	}
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		t := templates[i]
		lines = append(lines, fmt.Sprintf("#%d:%s (%s)", t.ID, t.Tag, t.GameVersion))
	}
	msg := "templates: " + strings.Join(lines, ", ")
	if len(templates) > limit {
		msg += fmt.Sprintf(" ... and %d more", len(templates)-limit)
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: msg}
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
		Alias:       req.WorldAlias,
		OwnerID:     actor.ID,
		SourceType:  "empty",
		GameVersion: version,
		AccessMode:  "privacy",
		Status:      string(worker.StatusWaiting),
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
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	instanceID := inst.ID
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
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	instanceID := inst.ID
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
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	instanceID := inst.ID
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

func (s *ServiceI) handleWorldList(ctx context.Context, actor pgsql.User) (int, WorldCommandResponse) {
	list, err := s.repos.MapInstance.ListByOwner(ctx, actor.ID)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "list worlds failed"}
	}
	if len(list) == 0 {
		return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "no worlds"}
	}
	items := make([]string, 0, len(list))
	for _, inst := range list {
		items = append(items, fmt.Sprintf("%d:%s:%s", inst.ID, inst.Alias, inst.Status))
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: strings.Join(items, ", ")}
}

func (s *ServiceI) handleWorldSetAccess(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if req.AccessMode != "public" && req.AccessMode != "privacy" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "access_mode must be public or privacy"}
	}
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	if !canManage(actor, inst.OwnerID) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "permission denied"}
	}
	inst.AccessMode = req.AccessMode
	if err := s.repos.MapInstance.Update(ctx, inst); err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "update access mode failed"}
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "access mode updated"}
}

func (s *ServiceI) handleWorldInfo(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	members, err := s.repos.InstanceMember.ListByInstance(ctx, inst.ID)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "load members failed"}
	}
	names := make([]string, 0, len(members))
	for _, m := range members {
		u, err := s.repos.User.Read(ctx, m.UserID)
		if err == nil {
			names = append(names, u.MCName)
		}
		if len(names) >= 10 {
			break
		}
	}
	msg := fmt.Sprintf("id=%d alias=%s status=%s access=%s members=%d", inst.ID, inst.Alias, inst.Status, inst.AccessMode, len(members))
	if len(names) > 0 {
		msg += " [" + strings.Join(names, ",") + "]"
	}
	if !canManage(actor, inst.OwnerID) {
		// non-owner can still read basic info
		return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: msg}
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: msg}
}

func (s *ServiceI) handleWorldJoin(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	if inst.Status != string(worker.StatusOn) {
		return http.StatusConflict, WorldCommandResponse{Status: "error", Message: "instance is not On"}
	}
	if !s.canJoinInstance(ctx, actor, inst) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "join denied"}
	}
	if err := s.sendPlayerToInstance(ctx, actor.MCName, inst.ID); err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "send player failed"}
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("joining #%d:%s", inst.ID, inst.Alias)}
}

func (s *ServiceI) handleInstanceList(ctx context.Context, actor pgsql.User) (int, WorldCommandResponse) {
	if !isAdmin(actor) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}
	list, err := s.repos.MapInstance.List(ctx)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "list instances failed"}
	}
	if len(list) == 0 {
		return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "no instances"}
	}
	items := make([]string, 0, len(list))
	for _, inst := range list {
		items = append(items, fmt.Sprintf("%d:%s:%s", inst.ID, inst.Alias, inst.Status))
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: strings.Join(items, ", ")}
}

func (s *ServiceI) ensureActor(ctx context.Context, actorUUID, actorName string) (pgsql.User, error) {
	actorUUID = strings.TrimSpace(actorUUID)
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "unknown"
	}

	u, err := s.repos.User.ReadByUUID(ctx, actorUUID)
	if err == nil {
		if u.MCName != actorName {
			oldName := u.MCName
			u.MCName = actorName
			if upErr := s.repos.User.Update(ctx, u); upErr != nil {
				s.logger.Warnf("ensure_actor rename failed user_id=%d uuid=%s old=%s new=%s err=%v", u.ID, actorUUID, oldName, actorName, upErr)
			} else {
				s.logger.Infof("ensure_actor renamed user_id=%d uuid=%s old=%s new=%s", u.ID, actorUUID, oldName, actorName)
			}
		}
		s.logger.Infof("ensure_actor hit_by_uuid user_id=%d actor=%s uuid=%s role=%s", u.ID, actorName, actorUUID, u.ServerRole)
		return u, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return pgsql.User{}, err
	}

	byName, nameErr := s.repos.User.ReadByName(ctx, actorName)
	if nameErr == nil {
		oldUUID := byName.MCUUID
		byName.MCUUID = actorUUID
		if upErr := s.repos.User.Update(ctx, byName); upErr != nil {
			return pgsql.User{}, upErr
		}
		s.logger.Warnf("ensure_actor rebound_uuid user_id=%d actor=%s old_uuid=%s new_uuid=%s", byName.ID, actorName, oldUUID, actorUUID)
		return byName, nil
	}
	if !errors.Is(nameErr, sql.ErrNoRows) {
		return pgsql.User{}, nameErr
	}

	id, err := s.repos.User.Create(ctx, pgsql.User{
		MCUUID:     actorUUID,
		MCName:     actorName,
		ServerRole: "user",
	})
	if err != nil {
		return pgsql.User{}, err
	}
	created, err := s.repos.User.Read(ctx, id)
	if err != nil {
		return pgsql.User{}, err
	}
	s.logger.Infof("ensure_actor created user_id=%d actor=%s uuid=%s role=%s", created.ID, actorName, actorUUID, created.ServerRole)
	return created, nil
}

func canManage(actor pgsql.User, ownerID int64) bool {
	return actor.ServerRole == "admin" || actor.ID == ownerID
}

func isAdmin(actor pgsql.User) bool {
	return actor.ServerRole == "admin"
}

func isOpOnlyAction(action string) bool {
	switch action {
	case "request_approve", "request_reject", "instance_list":
		return true
	default:
		return false
	}
}

func (s *ServiceI) canJoinInstance(ctx context.Context, actor pgsql.User, inst pgsql.MapInstance) bool {
	if actor.ServerRole == "admin" || actor.ID == inst.OwnerID {
		return true
	}
	if strings.EqualFold(inst.AccessMode, "public") {
		return true
	}
	members, err := s.repos.InstanceMember.ListByInstance(ctx, inst.ID)
	if err != nil {
		return false
	}
	for _, m := range members {
		if m.UserID == actor.ID {
			return true
		}
	}
	return false
}

func (s *ServiceI) resolveInstance(ctx context.Context, ident string) (pgsql.MapInstance, error) {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return pgsql.MapInstance{}, sql.ErrNoRows
	}
	if id, err := parseInstanceID(ident); err == nil {
		return s.repos.MapInstance.Read(ctx, id)
	}
	return s.repos.MapInstance.ReadByAlias(ctx, ident)
}

func parseInstanceID(alias string) (int64, error) {
	s := strings.TrimSpace(alias)
	s = strings.TrimPrefix(s, "inst-")
	return strconv.ParseInt(s, 10, 64)
}

func (s *ServiceI) resolveUserRequest(ctx context.Context, ident string) (pgsql.UserRequest, error) {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return pgsql.UserRequest{}, sql.ErrNoRows
	}
	if id, err := strconv.ParseInt(ident, 10, 64); err == nil {
		return s.repos.UserRequest.Read(ctx, id)
	}
	return s.repos.UserRequest.ReadByRequestID(ctx, ident)
}

func (s *ServiceI) resolveTemplate(ctx context.Context, ident string) (pgsql.MapTemplate, error) {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return pgsql.MapTemplate{}, sql.ErrNoRows
	}
	if id, err := strconv.ParseInt(ident, 10, 64); err == nil {
		return s.repos.MapTemplate.Read(ctx, id)
	}
	return s.repos.MapTemplate.ReadByTag(ctx, ident)
}

func buildOwnedAlias(ownerName string, rawAlias string) string {
	owner := strings.TrimSpace(ownerName)
	alias := strings.TrimSpace(rawAlias)
	if owner == "" {
		owner = "user"
	}
	if alias == "" {
		alias = "world"
	}
	return owner + "_" + alias
}

func (s *ServiceI) notifyLobbyAdminsRequestCreated(
	ctx context.Context,
	actorName string,
	worldAlias string,
	templateName string,
	requestNo int64,
	requestID string,
) error {
	if s.lobbyTapURL == "" {
		return nil
	}
	conn, err := servertap.NewConnectorWithAuth(s.lobbyTapURL, 5*time.Second, s.serverTapAuthName, s.serverTapKey)
	if err != nil {
		return err
	}
	admins, err := s.repos.User.ListByRole(ctx, "admin")
	if err != nil {
		return err
	}
	if len(admins) == 0 {
		return nil
	}
	tpl := strings.TrimSpace(templateName)
	if tpl == "" {
		tpl = "empty"
	}
	msg := fmt.Sprintf("[MCMM] req#%d from %s world=%s template=%s", requestNo, actorName, worldAlias, tpl)
	names := make([]string, 0, len(admins))
	for _, a := range admins {
		names = append(names, a.MCName)
	}
	if err := s.notifyPlayersViaLobbyTap(ctx, conn, names, msg); err != nil {
		s.logger.Warnf("notify admins failed req=%d/%s err=%v", requestNo, requestID, err)
	}
	return nil
}

func (s *ServiceI) notifyApproveResult(ctx context.Context, ur pgsql.UserRequest, success bool, instanceID int64, reason string) {
	if s.lobbyTapURL == "" {
		return
	}
	conn, err := servertap.NewConnectorWithAuth(s.lobbyTapURL, 5*time.Second, s.serverTapAuthName, s.serverTapKey)
	if err != nil {
		return
	}
	admins, err := s.repos.User.ListByRole(ctx, "admin")
	if err != nil {
		return
	}
	names := make([]string, 0, len(admins)+1)
	for _, a := range admins {
		names = append(names, a.MCName)
	}
	if owner, err := s.repos.User.Read(ctx, ur.ActorUserID); err == nil {
		names = append(names, owner.MCName)
	}
	msg := ""
	if success {
		msg = fmt.Sprintf("[MCMM] req#%d approved and started: instance=%d", ur.ID, instanceID)
	} else {
		msg = fmt.Sprintf("[MCMM] req#%d failed: %s", ur.ID, reason)
	}
	_ = s.notifyPlayersViaLobbyTap(ctx, conn, names, msg)
}

func (s *ServiceI) notifyPlayersViaLobbyTap(ctx context.Context, conn *servertap.Connector, names []string, msg string) error {
	sent := map[string]struct{}{}
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := sent[key]; ok {
			continue
		}
		cmd := servertap.NewCommandBuilder("tell").Arg(name).RawArg(msg).Build()
		if _, err := conn.Execute(ctx, servertap.ExecuteRequest{Command: cmd}); err != nil {
			s.logger.Warnf("notify player failed player=%s err=%v", name, err)
			continue
		}
		sent[key] = struct{}{}
	}
	return nil
}

func (s *ServiceI) sendPlayerToInstance(ctx context.Context, playerName string, instanceID int64) error {
	if s.lobbyTapURL == "" {
		return fmt.Errorf("lobby servertap not configured")
	}
	conn, err := servertap.NewConnectorWithAuth(s.lobbyTapURL, 5*time.Second, s.serverTapAuthName, s.serverTapKey)
	if err != nil {
		return err
	}
	serverName := fmt.Sprintf("mcmm-inst-%d", instanceID)
	cmd := servertap.NewCommandBuilder("send").Arg(playerName).Arg(serverName).Build()
	_, err = conn.Execute(ctx, servertap.ExecuteRequest{Command: cmd})
	return err
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
