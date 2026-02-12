package cmdreceiver

import (
	"encoding/json"
	"net/http"
	"strings"
)

type WorldCommandRequest struct {
	Action     string `json:"action"`
	ActorUUID  string `json:"actor_uuid"`
	ActorName  string `json:"actor_name"`
	WorldAlias string `json:"world_alias"`
	Target     string `json:"target_name"`
	RequestID  string `json:"request_id"`
}

type WorldCommandResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type HandlerI struct{}

func NewHandlerI() *HandlerI {
	return &HandlerI{}
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
		Action:     strings.TrimSpace(r.FormValue("action")),
		ActorUUID:  strings.TrimSpace(r.FormValue("actor_uuid")),
		ActorName:  strings.TrimSpace(r.FormValue("actor_name")),
		WorldAlias: strings.TrimSpace(r.FormValue("world_alias")),
		Target:     strings.TrimSpace(r.FormValue("target_name")),
		RequestID:  strings.TrimSpace(r.FormValue("request_id")),
	}

	if req.Action == "" || req.ActorUUID == "" {
		writeJSON(w, http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "missing required fields"})
		return
	}

	switch req.Action {
	case "create":
		// create does not require world alias and target.
	case "delete":
		if req.WorldAlias == "" {
			writeJSON(w, http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "world_alias is required for delete"})
			return
		}
	case "member_add", "member_remove":
		if req.WorldAlias == "" || req.Target == "" {
			writeJSON(w, http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "world_alias and target_name are required for member action"})
			return
		}
	default:
		writeJSON(w, http.StatusBadRequest, WorldCommandResponse{Status: "error", Message: "unsupported action"})
		return
	}

	// TODO: wire to service layer + ownership checks + LP operations.
	if req.Action == "create" {
		// TODO: generate UUID-based identity code and return one-time URL valid for 10 minutes.
		writeJSON(w, http.StatusOK, WorldCommandResponse{
			Status:  "accepted",
			Message: "create request accepted (scaffold): issue identity code + url in service layer",
		})
		return
	}
	writeJSON(w, http.StatusOK, WorldCommandResponse{Status: "accepted", Message: "world command accepted (scaffold)"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
