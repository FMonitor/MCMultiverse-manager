package cmdreceiver

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
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
	instanceTapPattern string
	proxyBridgeURL     string
	proxyAuthHeader    string
	proxyAuthToken     string
	logger             interface {
		Infof(string, ...any)
		Warnf(string, ...any)
		Errorf(string, ...any)
	}
}

var onlineListRegex = regexp.MustCompile(`(?i)players online:\s*(.+)$`)

func NewServiceI(
	repos pgsql.Repos,
	w worker.Worker,
	defaultGameVersion string,
	lobbyTapURL string,
	serverTapAuthName string,
	serverTapKey string,
	instanceTapPattern string,
	proxyBridgeURL string,
	proxyAuthHeader string,
	proxyAuthToken string,
) *ServiceI {
	if defaultGameVersion == "" {
		defaultGameVersion = "1.21.1"
	}
	if strings.TrimSpace(proxyAuthHeader) == "" {
		proxyAuthHeader = "Authorization"
	}
	return &ServiceI{
		repos:              repos,
		worker:             w,
		defaultGameVersion: defaultGameVersion,
		lobbyTapURL:        strings.TrimSpace(lobbyTapURL),
		serverTapAuthName:  strings.TrimSpace(serverTapAuthName),
		serverTapKey:       strings.TrimSpace(serverTapKey),
		instanceTapPattern: strings.TrimSpace(instanceTapPattern),
		proxyBridgeURL:     strings.TrimRight(strings.TrimSpace(proxyBridgeURL), "/"),
		proxyAuthHeader:    strings.TrimSpace(proxyAuthHeader),
		proxyAuthToken:     strings.TrimSpace(proxyAuthToken),
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
	case "world_on":
		return s.handleWorldPower(ctx, req, actor, true)
	case "world_off":
		return s.handleWorldPower(ctx, req, actor, false)
	case "lobby_join":
		return s.handleLobbyJoin(ctx, actor)
	case "world_remove", "delete":
		return s.handleDelete(ctx, req, actor)
	case "member_add":
		return s.handleMemberAdd(ctx, req, actor)
	case "member_remove":
		return s.handleMemberRemove(ctx, req, actor)
	case "player_invite":
		return s.handleMemberAdd(ctx, req, actor)
	case "player_reject":
		return s.handleMemberRemove(ctx, req, actor)
	case "player_list":
		return s.handlePlayerList(ctx)
	case "instance_list":
		return s.handleInstanceList(ctx, actor)
	case "instance_create":
		return s.handleInstanceCreate(ctx, req, actor)
	case "instance_stop":
		return s.handleInstancePower(ctx, req, actor, false)
	case "instance_on":
		return s.handleInstancePower(ctx, req, actor, true)
	case "instance_off":
		return s.handleInstancePower(ctx, req, actor, false)
	case "instance_remove":
		return s.handleInstanceRemove(ctx, req, actor)
	case "instance_lockdown":
		return s.handleInstanceLockdown(ctx, req, actor)
	case "instance_unlock":
		return s.handleInstanceUnlock(ctx, req, actor)
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
	templateLabel := "empty"
	if req.TemplateName != "" {
		template, err = s.resolveTemplate(ctx, req.TemplateName)
		if err != nil {
			return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "template not found"}
		}
		templateID = sql.NullInt64{Int64: template.ID, Valid: true}
		templateLabel = fmt.Sprintf("#%d %s", template.ID, template.Tag)
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
		Message: fmt.Sprintf(
			"request created: #%d world=%s template=%s",
			requestNo,
			finalAlias,
			templateLabel,
		),
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
	return http.StatusAccepted, WorldCommandResponse{
		Status: "accepted",
		Message: fmt.Sprintf(
			"request #%d approved, creating world=%s template=%s",
			ur.ID,
			strOrDefault(ur.RequestedAlias, "-"),
			s.resolveTemplateDisplayByID(ctx, ur.TemplateID),
		),
	}
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
			s.notifyApproveResult(ctx, ur, false, 0, "template not found", ur.RequestedAlias.String, "unknown")
			return
		}
		instance.SourceType = "template"
		instance.GameVersion = template.GameVersion
	}

	instanceID, err := s.repos.MapInstance.Create(ctx, instance)
	if err != nil {
		_ = s.repos.UserRequest.MarkRequestResult(ctx, ur.RequestID, "failed", json.RawMessage(`{"step":"create_instance_row"}`), sql.NullString{String: "db_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
		s.notifyApproveResult(ctx, ur, false, 0, "create instance failed", ur.RequestedAlias.String, displayTemplate(template.Tag))
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
			s.notifyApproveResult(ctx, ur, false, instanceID, "start template failed", instance.Alias, displayTemplate(template.Tag))
			return
		}
	} else {
		if err := s.worker.StartEmpty(ctx, instanceID, instance.GameVersion); err != nil {
			_ = s.repos.UserRequest.MarkRequestResult(ctx, ur.RequestID, "failed", json.RawMessage(`{"step":"start_empty"}`), sql.NullString{String: "worker_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
			s.notifyApproveResult(ctx, ur, false, instanceID, "start empty failed", instance.Alias, "empty")
			return
		}
	}
	_ = s.repos.UserRequest.MarkRequestResult(ctx, ur.RequestID, "succeeded", json.RawMessage(fmt.Sprintf(`{"instance_id":%d}`, instanceID)), sql.NullString{}, sql.NullString{})
	s.notifyApproveResult(ctx, ur, true, instanceID, "", instance.Alias, displayTemplate(template.Tag))
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

	ur, _, err := s.repos.UserRequest.CreateAcceptedIfNotExists(
		ctx,
		req.RequestID,
		"delete_instance",
		sql.NullInt64{Int64: actor.ID, Valid: true},
		sql.NullInt64{Int64: instanceID, Valid: true},
	)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "delete request failed"}
	}
	ur.Status = "processing"
	_ = s.repos.UserRequest.Update(ctx, ur)

	go func(requestID string, id int64, alias string) {
		runCtx := context.Background()
		if err := s.worker.StopAndArchive(runCtx, id); err != nil {
			s.logger.Errorf("world remove failed instance=%d alias=%s err=%v", id, alias, err)
			_ = s.repos.UserRequest.MarkRequestResult(runCtx, requestID, "failed", json.RawMessage(`{"step":"stop_archive"}`), sql.NullString{String: "worker_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
			return
		}
		_ = s.repos.UserRequest.MarkRequestResult(runCtx, requestID, "succeeded", json.RawMessage(fmt.Sprintf(`{"instance_id":%d}`, id)), sql.NullString{}, sql.NullString{})
	}(req.RequestID, instanceID, inst.Alias)

	return http.StatusAccepted, WorldCommandResponse{
		Status:  "accepted",
		Message: fmt.Sprintf("world remove started: #%d:%s", inst.ID, inst.Alias),
	}
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
			_ = s.updateInstanceWhitelist(ctx, instanceID, target.MCName, true)
			return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "already a member"}
		}
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "add member failed"}
	}
	_ = s.updateInstanceWhitelist(ctx, instanceID, target.MCName, true)
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
	_ = s.updateInstanceWhitelist(ctx, instanceID, target.MCName, false)
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "member removed"}
}

func (s *ServiceI) handleWorldList(ctx context.Context, actor pgsql.User) (int, WorldCommandResponse) {
	all, err := s.repos.MapInstance.List(ctx)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "list worlds failed"}
	}
	members, err := s.repos.InstanceMember.ListByUser(ctx, actor.ID)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "list world members failed"}
	}
	memberSet := make(map[int64]string, len(members))
	for _, m := range members {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role == "" {
			role = "member"
		}
		memberSet[m.InstanceID] = role
	}

	type worldView struct {
		id     int64
		alias  string
		status string
		role   string
	}
	picked := make(map[int64]worldView)
	for _, inst := range all {
		if inst.Status != string(worker.StatusOn) && inst.Status != string(worker.StatusOff) {
			continue
		}
		role := ""
		switch {
		case isAdmin(actor):
			role = "admin"
		case inst.OwnerID == actor.ID:
			role = "owner"
		case memberSet[inst.ID] != "":
			role = "member"
		case strings.EqualFold(inst.AccessMode, "public") && inst.Status == string(worker.StatusOn):
			role = "public"
		}
		if role == "" {
			continue
		}
		picked[inst.ID] = worldView{
			id:     inst.ID,
			alias:  inst.Alias,
			status: inst.Status,
			role:   role,
		}
	}

	if len(picked) == 0 {
		return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "no worlds"}
	}
	rows := make([]worldView, 0, len(picked))
	for _, v := range picked {
		rows = append(rows, v)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })

	items := make([]string, 0, len(rows))
	for _, r := range rows {
		items = append(items, fmt.Sprintf("#%d:%s:%s(%s)", r.id, r.alias, r.status, r.role))
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

func (s *ServiceI) handleWorldPower(ctx context.Context, req WorldCommandRequest, actor pgsql.User, on bool) (int, WorldCommandResponse) {
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	if !canManage(actor, inst.OwnerID) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "permission denied"}
	}
	go func(id int64, alias string, ownerID int64, actorID int64) {
		runCtx := context.Background()
		var runErr error
		if on {
			runErr = s.worker.StartExisting(runCtx, id)
		} else {
			runErr = s.worker.StopOnly(runCtx, id)
		}
		if runErr != nil {
			s.logger.Errorf("world power failed instance=%d alias=%s on=%v err=%v", id, alias, on, runErr)
			s.notifyInstancePowerResult(runCtx, id, alias, ownerID, actorID, "world", on, false, runErr.Error())
			return
		}
		s.notifyInstancePowerResult(runCtx, id, alias, ownerID, actorID, "world", on, true, "")
	}(inst.ID, inst.Alias, inst.OwnerID, actor.ID)
	if on {
		return http.StatusAccepted, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("world start requested: #%d:%s", inst.ID, inst.Alias)}
	}
	return http.StatusAccepted, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("world stop requested: #%d:%s", inst.ID, inst.Alias)}
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

func (s *ServiceI) handleLobbyJoin(ctx context.Context, actor pgsql.User) (int, WorldCommandResponse) {
	if err := s.sendPlayerToServer(ctx, actor.MCName, "lobby"); err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "send player to lobby failed"}
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "returning to lobby"}
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

func (s *ServiceI) handleInstanceCreate(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if !isAdmin(actor) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}
	if req.WorldAlias == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "world_alias is required"}
	}
	finalAlias := buildOwnedAlias(actor.MCName, req.WorldAlias)
	if _, err := s.repos.MapInstance.ReadByAlias(ctx, finalAlias); err == nil {
		return http.StatusConflict, WorldCommandResponse{Status: "error", Message: "world_alias already exists"}
	}

	instance := pgsql.MapInstance{
		Alias:       finalAlias,
		OwnerID:     actor.ID,
		SourceType:  "empty",
		GameVersion: s.defaultGameVersion,
		AccessMode:  "privacy",
		Status:      string(worker.StatusWaiting),
	}
	var template pgsql.MapTemplate
	if req.TemplateName != "" {
		t, err := s.resolveTemplate(ctx, req.TemplateName)
		if err != nil {
			return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "template not found"}
		}
		template = t
		instance.TemplateID = sql.NullInt64{Int64: template.ID, Valid: true}
		instance.SourceType = "template"
		instance.GameVersion = template.GameVersion
	}

	instanceID, err := s.repos.MapInstance.Create(ctx, instance)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "create instance failed"}
	}
	_, _ = s.repos.InstanceMember.Create(ctx, pgsql.InstanceMember{InstanceID: instanceID, UserID: actor.ID, Role: "owner"})

	go func() {
		runCtx := context.Background()
		var runErr error
		if instance.TemplateID.Valid {
			runErr = s.worker.StartFromTemplate(runCtx, instanceID, template)
		} else {
			runErr = s.worker.StartEmpty(runCtx, instanceID, instance.GameVersion)
		}
		if runErr != nil {
			s.logger.Errorf("instance_create failed instance=%d alias=%s err=%v", instanceID, finalAlias, runErr)
			return
		}
		s.logger.Infof("instance_create done instance=%d alias=%s", instanceID, finalAlias)
	}()

	return http.StatusAccepted, WorldCommandResponse{
		Status: "accepted",
		Message: fmt.Sprintf(
			"instance creating: id=%d world=%s template=%s. join with: /mcmm world #%d:%s",
			instanceID,
			finalAlias,
			displayTemplate(template.Tag),
			instanceID,
			finalAlias,
		),
	}
}

func (s *ServiceI) handleInstancePower(ctx context.Context, req WorldCommandRequest, actor pgsql.User, on bool) (int, WorldCommandResponse) {
	if !isAdmin(actor) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	go func(id int64, alias string, ownerID int64, actorID int64) {
		runCtx := context.Background()
		var runErr error
		if on {
			runErr = s.worker.StartExisting(runCtx, id)
		} else {
			runErr = s.worker.StopOnly(runCtx, id)
		}
		if runErr != nil {
			s.logger.Errorf("instance power failed instance=%d alias=%s on=%v err=%v", id, alias, on, runErr)
			s.notifyInstancePowerResult(runCtx, id, alias, ownerID, actorID, "instance", on, false, runErr.Error())
			return
		}
		s.notifyInstancePowerResult(runCtx, id, alias, ownerID, actorID, "instance", on, true, "")
	}(inst.ID, inst.Alias, inst.OwnerID, actor.ID)
	if on {
		return http.StatusAccepted, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("instance start requested: #%d:%s", inst.ID, inst.Alias)}
	}
	return http.StatusAccepted, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("instance stop requested: #%d:%s", inst.ID, inst.Alias)}
}

func (s *ServiceI) notifyInstancePowerResult(
	ctx context.Context,
	instanceID int64,
	alias string,
	ownerID int64,
	actorID int64,
	scope string,
	on bool,
	success bool,
	reason string,
) {
	if s.lobbyTapURL == "" {
		return
	}
	conn, err := servertap.NewConnectorWithAuth(s.lobbyTapURL, 5*time.Second, s.serverTapAuthName, s.serverTapKey)
	if err != nil {
		return
	}
	names := make([]string, 0, 8)
	if u, err := s.repos.User.Read(ctx, ownerID); err == nil {
		names = append(names, u.MCName)
	}
	if u, err := s.repos.User.Read(ctx, actorID); err == nil {
		names = append(names, u.MCName)
	}
	if admins, err := s.repos.User.ListByRole(ctx, "admin"); err == nil {
		for _, a := range admins {
			names = append(names, a.MCName)
		}
	}
	op := "off"
	if on {
		op = "on"
	}
	msg := ""
	if success {
		msg = fmt.Sprintf("[MCMM] %s %s completed: #%d:%s", scope, op, instanceID, alias)
	} else {
		msg = fmt.Sprintf("[MCMM] %s %s failed: #%d:%s (%s)", scope, op, instanceID, alias, reason)
	}
	_ = s.notifyPlayersViaLobbyTap(ctx, conn, names, msg)
}

func (s *ServiceI) handleInstanceRemove(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if !isAdmin(actor) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	go func() {
		runCtx := context.Background()
		if err := s.worker.StopAndArchive(runCtx, inst.ID); err != nil {
			s.logger.Errorf("instance_remove failed instance=%d alias=%s err=%v", inst.ID, inst.Alias, err)
			return
		}
		s.logger.Infof("instance_remove done instance=%d alias=%s", inst.ID, inst.Alias)
	}()
	return http.StatusAccepted, WorldCommandResponse{
		Status:  "accepted",
		Message: fmt.Sprintf("instance remove started: #%d %s", inst.ID, inst.Alias),
	}
}

func (s *ServiceI) handleInstanceLockdown(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if !isAdmin(actor) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	inst.AccessMode = "lockdown"
	if err := s.repos.MapInstance.Update(ctx, inst); err != nil {
		s.logger.Errorf("instance lockdown update failed instance=%d alias=%s err=%v", inst.ID, inst.Alias, err)
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "instance lockdown failed"}
	}
	if err := s.kickNonAdminPlayers(ctx, inst.ID); err != nil {
		s.logger.Warnf("instance lockdown kick non-admin failed instance=%d alias=%s err=%v", inst.ID, inst.Alias, err)
	}
	return http.StatusOK, WorldCommandResponse{
		Status:  "accepted",
		Message: fmt.Sprintf("instance locked: #%d:%s", inst.ID, inst.Alias),
	}
}

func (s *ServiceI) handleInstanceUnlock(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if !isAdmin(actor) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}
	inst, err := s.resolveInstance(ctx, req.WorldAlias)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "instance not found"}
	}
	inst.AccessMode = "privacy"
	if err := s.repos.MapInstance.Update(ctx, inst); err != nil {
		s.logger.Errorf("instance unlock update failed instance=%d alias=%s err=%v", inst.ID, inst.Alias, err)
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "instance unlock failed"}
	}
	return http.StatusOK, WorldCommandResponse{
		Status:  "accepted",
		Message: fmt.Sprintf("instance unlocked: #%d:%s", inst.ID, inst.Alias),
	}
}

func (s *ServiceI) handlePlayerList(ctx context.Context) (int, WorldCommandResponse) {
	users, err := s.repos.User.List(ctx)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "list players failed"}
	}
	if len(users) == 0 {
		return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "no players"}
	}
	limit := len(users)
	if limit > 200 {
		limit = 200
	}
	names := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		if strings.TrimSpace(users[i].MCName) == "" {
			continue
		}
		names = append(names, users[i].MCName)
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "players: " + strings.Join(names, ", ")}
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
	if strings.EqualFold(inst.AccessMode, "lockdown") {
		return actor.ServerRole == "admin"
	}
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
	if id, ok := parseSharpNumericID(s); ok {
		return id, nil
	}
	s = strings.TrimPrefix(s, "inst-")
	return strconv.ParseInt(s, 10, 64)
}

func (s *ServiceI) resolveUserRequest(ctx context.Context, ident string) (pgsql.UserRequest, error) {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return pgsql.UserRequest{}, sql.ErrNoRows
	}
	if id, ok := parseSharpNumericID(ident); ok {
		return s.repos.UserRequest.Read(ctx, id)
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
	if id, ok := parseSharpNumericID(ident); ok {
		return s.repos.MapTemplate.Read(ctx, id)
	}
	if id, err := strconv.ParseInt(ident, 10, 64); err == nil {
		return s.repos.MapTemplate.Read(ctx, id)
	}
	return s.repos.MapTemplate.ReadByTag(ctx, ident)
}

func parseSharpNumericID(raw string) (int64, bool) {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "#") {
		return 0, false
	}
	s = strings.TrimPrefix(s, "#")
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	id, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
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

func (s *ServiceI) notifyApproveResult(
	ctx context.Context,
	ur pgsql.UserRequest,
	success bool,
	instanceID int64,
	reason string,
	worldAlias string,
	templateName string,
) {
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
		msg = fmt.Sprintf(
			"[MCMM] req#%d approved. world=%s template=%s instance=%d. Use /mcmm world #%d:%s to join",
			ur.ID,
			worldAlias,
			displayTemplate(templateName),
			instanceID,
			instanceID,
			worldAlias,
		)
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
	serverID := fmt.Sprintf("mcmm-inst-%d", instanceID)
	if s.proxyBridgeURL != "" {
		if err := s.proxyRegister(ctx, serverID, serverID, 25565); err != nil {
			return fmt.Errorf("proxy register failed: %w", err)
		}
		return s.sendPlayerToServer(ctx, playerName, serverID)
	}

	if s.lobbyTapURL == "" {
		return fmt.Errorf("lobby servertap not configured")
	}
	conn, err := servertap.NewConnectorWithAuth(s.lobbyTapURL, 5*time.Second, s.serverTapAuthName, s.serverTapKey)
	if err != nil {
		return err
	}
	cmd := servertap.NewCommandBuilder("send").Arg(playerName).Arg(serverID).Build()
	_, err = conn.Execute(ctx, servertap.ExecuteRequest{Command: cmd})
	return err
}

func (s *ServiceI) sendPlayerToServer(ctx context.Context, playerName, serverID string) error {
	if s.proxyBridgeURL == "" {
		return fmt.Errorf("proxy bridge not configured")
	}
	if err := s.proxySend(ctx, playerName, serverID); err != nil {
		return fmt.Errorf("proxy send failed: %w", err)
	}
	return nil
}

func (s *ServiceI) updateInstanceWhitelist(ctx context.Context, instanceID int64, playerName string, add bool) error {
	if strings.TrimSpace(s.instanceTapPattern) == "" {
		return nil
	}
	inst, err := s.repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		return err
	}
	// If container is not online, DB membership is enough; whitelist will be synced on next start.
	if inst.Status != string(worker.StatusOn) {
		return nil
	}
	tapURL := fmt.Sprintf(s.instanceTapPattern, instanceID)
	conn, err := servertap.NewConnectorWithAuth(tapURL, 5*time.Second, s.serverTapAuthName, s.serverTapKey)
	if err != nil {
		return err
	}
	cmd := "whitelist remove " + playerName
	if add {
		cmd = "whitelist add " + playerName
	}
	_, err = conn.Execute(ctx, servertap.ExecuteRequest{Command: cmd})
	if err != nil {
		s.logger.Warnf("whitelist update failed instance=%d add=%v player=%s err=%v", instanceID, add, playerName, err)
	}
	return err
}

func (s *ServiceI) kickNonAdminPlayers(ctx context.Context, instanceID int64) error {
	serverID := fmt.Sprintf("mcmm-inst-%d", instanceID)
	if s.proxyBridgeURL != "" {
		players, err := s.proxyListPlayersByServer(ctx, serverID)
		if err == nil && len(players) > 0 {
			for _, p := range players {
				u, err := s.repos.User.ReadByName(ctx, p)
				if err == nil && strings.EqualFold(u.ServerRole, "admin") {
					continue
				}
				if err := s.proxySend(ctx, p, "lobby"); err != nil {
					s.logger.Warnf("lockdown move to lobby failed instance=%d player=%s err=%v", instanceID, p, err)
				} else {
					s.logger.Infof("instance=%d moved player=%s to lobby due to lockdown", instanceID, p)
				}
			}
			return nil
		}
		if err != nil {
			s.logger.Warnf("proxy list players failed instance=%d err=%v", instanceID, err)
		}
	}

	if strings.TrimSpace(s.instanceTapPattern) == "" {
		return nil
	}
	tapURL := fmt.Sprintf(s.instanceTapPattern, instanceID)
	conn, err := servertap.NewConnectorWithAuth(tapURL, 5*time.Second, s.serverTapAuthName, s.serverTapKey)
	if err != nil {
		return err
	}
	resp, err := conn.Execute(ctx, servertap.ExecuteRequest{Command: "list"})
	if err != nil {
		return err
	}
	players := parseOnlinePlayers(resp.RawBody)
	for _, p := range players {
		u, err := s.repos.User.ReadByName(ctx, p)
		if err == nil && strings.EqualFold(u.ServerRole, "admin") {
			continue
		}
		cmd := servertap.NewCommandBuilder("kick").Arg(p).RawArg("Server is in lockdown").Build()
		if _, err := conn.Execute(ctx, servertap.ExecuteRequest{Command: cmd}); err != nil {
			s.logger.Warnf("kick failed instance=%d player=%s err=%v", instanceID, p, err)
		} else {
			s.logger.Infof("instance=%d kicked player=%s due to lockdown", instanceID, p)
		}
	}
	return nil
}

func (s *ServiceI) proxyListPlayersByServer(ctx context.Context, serverID string) ([]string, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.proxyBridgeURL+"/v1/proxy/players?server_id="+url.QueryEscape(serverID), nil)
	if err != nil {
		return nil, err
	}
	if s.proxyAuthHeader != "" && s.proxyAuthToken != "" {
		req.Header.Set(s.proxyAuthHeader, "Bearer "+s.proxyAuthToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		Status  string   `json:"status"`
		Players []string `json:"players"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Players, nil
}

func parseOnlinePlayers(raw string) []string {
	body := strings.TrimSpace(raw)
	if body == "" {
		return nil
	}
	m := onlineListRegex.FindStringSubmatch(body)
	if len(m) != 2 {
		return nil
	}
	seg := strings.TrimSpace(m[1])
	if seg == "" {
		return nil
	}
	parts := strings.Split(seg, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func (s *ServiceI) proxyRegister(ctx context.Context, serverID, host string, port int) error {
	values := url.Values{}
	values.Set("server_id", serverID)
	values.Set("host", host)
	values.Set("port", strconv.Itoa(port))
	return s.proxyPostForm(ctx, "/v1/proxy/register", values)
}

func (s *ServiceI) proxySend(ctx context.Context, playerName, serverID string) error {
	values := url.Values{}
	values.Set("player", playerName)
	values.Set("server_id", serverID)
	return s.proxyPostForm(ctx, "/v1/proxy/send", values)
}

func (s *ServiceI) proxyPostForm(ctx context.Context, path string, values url.Values) error {
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.proxyBridgeURL+path,
		strings.NewReader(values.Encode()),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if s.proxyAuthHeader != "" && s.proxyAuthToken != "" {
		req.Header.Set(s.proxyAuthHeader, "Bearer "+s.proxyAuthToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	s.logger.Infof("proxy_api %s ok status=%d body=%s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	return nil
}

func newUUIDLike() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func displayTemplate(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return "empty"
	}
	return n
}

func strOrDefault(v sql.NullString, d string) string {
	if v.Valid && strings.TrimSpace(v.String) != "" {
		return v.String
	}
	return d
}

func (s *ServiceI) resolveTemplateDisplayByID(ctx context.Context, id sql.NullInt64) string {
	if !id.Valid {
		return "empty"
	}
	t, err := s.repos.MapTemplate.Read(ctx, id.Int64)
	if err != nil {
		return "unknown"
	}
	return fmt.Sprintf("#%d:%s", t.ID, t.Tag)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
