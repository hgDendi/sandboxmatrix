package server

import (
	"net/http"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/quota"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// --------------------------------------------------------------------
// Team handlers
// --------------------------------------------------------------------

type createTeamRequest struct {
	Name        string                 `json:"name"`
	DisplayName string                 `json:"displayName,omitempty"`
	Members     []v1alpha1.TeamMember  `json:"members,omitempty"`
	Quota       v1alpha1.ResourceQuota `json:"quota,omitempty"`
}

func handleCreateTeam(teams state.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTeamRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			errorResponse(w, http.StatusBadRequest, "name is required")
			return
		}

		// Check if team already exists.
		if _, err := teams.Get(req.Name); err == nil {
			errorResponse(w, http.StatusConflict, "team already exists")
			return
		}

		team := &v1alpha1.Team{
			Name:        req.Name,
			DisplayName: req.DisplayName,
			Members:     req.Members,
			Quota:       req.Quota,
			CreatedAt:   time.Now(),
		}
		if team.Members == nil {
			team.Members = []v1alpha1.TeamMember{}
		}

		if err := teams.Save(team); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, team)
	}
}

func handleListTeams(teams state.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		list, err := teams.List()
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		if list == nil {
			list = []*v1alpha1.Team{}
		}
		jsonResponse(w, http.StatusOK, list)
	}
}

func handleGetTeam(teams state.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "team name is required")
			return
		}
		team, err := teams.Get(name)
		if err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, team)
	}
}

type updateTeamRequest struct {
	DisplayName *string                 `json:"displayName,omitempty"`
	Quota       *v1alpha1.ResourceQuota `json:"quota,omitempty"`
}

func handleUpdateTeam(teams state.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "team name is required")
			return
		}

		team, err := teams.Get(name)
		if err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}

		var req updateTeamRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.DisplayName != nil {
			team.DisplayName = *req.DisplayName
		}
		if req.Quota != nil {
			team.Quota = *req.Quota
		}

		if err := teams.Save(team); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, team)
	}
}

func handleDeleteTeam(teams state.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "team name is required")
			return
		}

		if err := teams.Delete(name); err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
	}
}

func handleGetTeamUsage(teams state.TeamStore, qm *quota.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "team name is required")
			return
		}

		// Verify team exists.
		if _, err := teams.Get(name); err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}

		usage, err := qm.GetUsage(name)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, usage)
	}
}

func handleListTeamMembers(teams state.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "team name is required")
			return
		}

		team, err := teams.Get(name)
		if err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, team.Members)
	}
}

type addMemberRequest struct {
	UserName string        `json:"userName"`
	Role     v1alpha1.Role `json:"role"`
}

func handleAddTeamMember(teams state.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "team name is required")
			return
		}

		team, err := teams.Get(name)
		if err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}

		var req addMemberRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.UserName == "" {
			errorResponse(w, http.StatusBadRequest, "userName is required")
			return
		}
		if req.Role == "" {
			req.Role = v1alpha1.RoleViewer
		}

		// Check if member already exists.
		for _, m := range team.Members {
			if m.UserName == req.UserName {
				errorResponse(w, http.StatusConflict, "member already exists in team")
				return
			}
		}

		team.Members = append(team.Members, v1alpha1.TeamMember{
			UserName: req.UserName,
			Role:     req.Role,
		})

		if err := teams.Save(team); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, team)
	}
}

func handleRemoveTeamMember(teams state.TeamStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		user := r.PathValue("user")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "team name is required")
			return
		}
		if user == "" {
			errorResponse(w, http.StatusBadRequest, "user name is required")
			return
		}

		team, err := teams.Get(name)
		if err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}

		found := false
		members := make([]v1alpha1.TeamMember, 0, len(team.Members))
		for _, m := range team.Members {
			if m.UserName == user {
				found = true
				continue
			}
			members = append(members, m)
		}

		if !found {
			errorResponse(w, http.StatusNotFound, "member not found in team")
			return
		}

		team.Members = members
		if err := teams.Save(team); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, team)
	}
}
