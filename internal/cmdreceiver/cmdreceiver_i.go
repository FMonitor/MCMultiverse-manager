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
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "load actor failed"}
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
	user, err := s.ensureActor(ctx, actorUUID, actorName)
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "upsert user failed"}
	}
	if user.MCName != actorName {
		user.MCName = actorName
		if err := s.repos.User.Update(ctx, user); err != nil {
			return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "update player name failed"}
		}
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("player synced id=%d", user.ID)}
}

func (s *ServiceI) handleRequestCreate(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if req.TemplateName == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "template_name is required"}
	}
	if req.WorldAlias == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "world_alias is required"}
	}
	if req.RequestID == "" {
		req.RequestID = newUUIDLike()
	}

	if _, err := s.repos.MapInstance.ReadByAlias(ctx, req.WorldAlias); err == nil {
		return http.StatusConflict, WorldCommandResponse{Status: "error", Message: "world_alias already exists"}
	}

	template, err := s.repos.MapTemplate.ReadByTag(ctx, req.TemplateName)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "template not found"}
	}

	ur, err := s.repos.UserRequest.ReadByRequestID(ctx, req.RequestID)
	if err == nil {
		return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("duplicate request_id, current status=%s", ur.Status)}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "read request failed"}
	}

	_, err = s.repos.UserRequest.Create(ctx, pgsql.UserRequest{
		RequestID:      req.RequestID,
		RequestType:    "world_create",
		ActorUserID:    actor.ID,
		TemplateID:     sql.NullInt64{Int64: template.ID, Valid: true},
		RequestedAlias: sql.NullString{String: req.WorldAlias, Valid: true},
		Status:         "pending",
		ResponsePayload: json.RawMessage(
			fmt.Sprintf(`{"template":"%s","world_alias":"%s"}`, template.Tag, req.WorldAlias),
		),
	})
	if err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "create request failed"}
	}

	return http.StatusOK, WorldCommandResponse{
		Status:  "accepted",
		Message: fmt.Sprintf("request created, request_id=%s", req.RequestID),
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
		out = append(out, fmt.Sprintf("%s:%s", r.RequestID, r.Status))
	}
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: strings.Join(out, ", ")}
}

func (s *ServiceI) handleRequestApprove(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if !isAdmin(actor) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}
	if req.RequestID == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "request_id is required"}
	}
	ur, err := s.repos.UserRequest.ReadByRequestID(ctx, req.RequestID)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "request not found"}
	}
	if ur.Status != "pending" {
		return http.StatusConflict, WorldCommandResponse{Status: "error", Message: fmt.Sprintf("request status is %s", ur.Status)}
	}
	if ur.RequestType != "world_create" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "request_type is not world_create"}
	}
	if !ur.TemplateID.Valid || !ur.RequestedAlias.Valid {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "request payload incomplete"}
	}

	template, err := s.repos.MapTemplate.Read(ctx, ur.TemplateID.Int64)
	if err != nil {
		return http.StatusNotFound, WorldCommandResponse{Status: "error", Message: "template not found"}
	}
	ur.Status = "processing"
	ur.ReviewedByUserID = sql.NullInt64{Int64: actor.ID, Valid: true}
	ur.TargetInstanceID = sql.NullInt64{}
	if err := s.repos.UserRequest.Update(ctx, ur); err != nil {
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "update request failed"}
	}

	instanceID, err := s.repos.MapInstance.Create(ctx, pgsql.MapInstance{
		Alias:       ur.RequestedAlias.String,
		OwnerID:     ur.ActorUserID,
		TemplateID:  ur.TemplateID,
		SourceType:  "template",
		GameVersion: template.GameVersion,
		AccessMode:  "privacy",
		Status:      string(worker.StatusWaiting),
	})
	if err != nil {
		_ = s.repos.UserRequest.MarkRequestResult(ctx, req.RequestID, "failed", json.RawMessage(`{"step":"create_instance_row"}`), sql.NullString{String: "db_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "create instance failed"}
	}
	_, _ = s.repos.InstanceMember.Create(ctx, pgsql.InstanceMember{
		InstanceID: instanceID,
		UserID:     ur.ActorUserID,
		Role:       "owner",
	})

	if err := s.worker.StartFromTemplate(ctx, instanceID, template); err != nil {
		_ = s.repos.UserRequest.MarkRequestResult(ctx, req.RequestID, "failed", json.RawMessage(`{"step":"start_template"}`), sql.NullString{String: "worker_error", Valid: true}, sql.NullString{String: err.Error(), Valid: true})
		return http.StatusInternalServerError, WorldCommandResponse{Status: "error", Message: "worker start failed"}
	}
	_ = s.repos.UserRequest.MarkRequestResult(ctx, req.RequestID, "succeeded", json.RawMessage(fmt.Sprintf(`{"instance_id":%d}`, instanceID)), sql.NullString{}, sql.NullString{})
	return http.StatusOK, WorldCommandResponse{Status: "accepted", Message: fmt.Sprintf("request approved, instance_id=%d", instanceID)}
}

func (s *ServiceI) handleRequestReject(ctx context.Context, req WorldCommandRequest, actor pgsql.User) (int, WorldCommandResponse) {
	if !isAdmin(actor) {
		return http.StatusForbidden, WorldCommandResponse{Status: "error", Message: "op only"}
	}
	if req.RequestID == "" {
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "request_id is required"}
	}
	ur, err := s.repos.UserRequest.ReadByRequestID(ctx, req.RequestID)
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
		return http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "request_id is required"}
	}
	ur, err := s.repos.UserRequest.ReadByRequestID(ctx, req.RequestID)
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
		lines = append(lines, fmt.Sprintf("%s (%s)", t.Tag, t.GameVersion))
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

func isAdmin(actor pgsql.User) bool {
	return actor.ServerRole == "admin"
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
