package handler_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"dougdomingos.com/aegis/internal/api/handler"
	groupErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
)

// ============================================================================
// CreateGroup (POST /groups)
// ============================================================================

func TestGroupHandler_CreateGroup_WithValidPayload_ReturnsStatus201(t *testing.T) {
	payload := schemas.CreateGroupSchema{Name: "lab1"}
	mock := &mockGroupService{
		createFn: func(_ context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
			return &schemas.GroupOutputSchema{Name: p.Name}, nil
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPost, "/groups", payload)
	assertStatusCode(t, rec, http.StatusCreated)
}

func TestGroupHandler_CreateGroup_WithEmptyName_ReturnsStatus400(t *testing.T) {
	payload := schemas.CreateGroupSchema{Name: ""}
	mock := &mockGroupService{
		createFn: func(_ context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNameRequired
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPost, "/groups", payload)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestGroupHandler_CreateGroup_WithExistentName_ReturnsStatus409(t *testing.T) {
	payload := schemas.CreateGroupSchema{Name: "lab1"}
	mock := &mockGroupService{
		createFn: func(_ context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNameAlreadyExists
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPost, "/groups", payload)
	assertStatusCode(t, rec, http.StatusConflict)
}

func TestGroupHandler_CreateGroup_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockGroupService{
		createFn: func(_ context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
			return nil, errors.New("malformed payload")
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPost, "/groups", "invalid json")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestGroupHandler_CreateGroup_WithEmptyPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockGroupService{
		createFn: func(_ context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
			return nil, errors.New("empty payload")
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPost, "/groups", "{}")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// GetGroupByName (GET /groups/{name})
// ============================================================================

func TestGroupHandler_GetGroupByName_WithExistentGroup_ReturnsStatus200(t *testing.T) {
	reqUrl := fmt.Sprintf("/groups/%s", "lab1")
	mock := &mockGroupService{
		getByNameFn: func(_ context.Context, p schemas.GetGroupByNameSchema) (*schemas.GroupOutputSchema, error) {
			return &schemas.GroupOutputSchema{Name: p.Name}, nil
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodGet, reqUrl, nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestGroupHandler_GetGroupByName_WithNonExistentGroup_ReturnsStatus404(t *testing.T) {
	reqUrl := fmt.Sprintf("/groups/%s", "unknown")
	mock := &mockGroupService{
		getByNameFn: func(_ context.Context, p schemas.GetGroupByNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNotFound
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodGet, reqUrl, nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

// ============================================================================
// List (GET /groups)
// ============================================================================

func TestGroupHandler_ListGroups_WithSeededGroups_ReturnsStatus200(t *testing.T) {
	mock := &mockGroupService{
		listFn: func(_ context.Context) ([]schemas.GroupOutputSchema, error) {
			return []schemas.GroupOutputSchema{
					{ID: 1, Name: "lab1"},
					{ID: 2, Name: "lab2"}},
				nil
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodGet, "/groups", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestGroupHandler_ListGroups_WithoutSeededGroups_ReturnsStatus200(t *testing.T) {
	mock := &mockGroupService{
		listFn: func(_ context.Context) ([]schemas.GroupOutputSchema, error) {
			return []schemas.GroupOutputSchema{}, nil
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodGet, "/groups", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

// ============================================================================
// Rename (PATCH /groups/rename)
// ============================================================================

func TestGroupHandler_RenameGroup_WithValidPayload_ReturnsStatus200(t *testing.T) {
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "lab1", NewName: "new name"}
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return &schemas.GroupOutputSchema{Name: "new name"}, nil
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPatch, "/groups/rename", payload)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestGroupHandler_RenameGroup_WithoutTargetGroupName_ReturnsStatus400(t *testing.T) {
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "", NewName: "new name"}
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNameRequired
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPatch, "/groups/rename", payload)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestGroupHandler_RenameGroup_WithoutNewName_ReturnsStatus400(t *testing.T) {
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "lab1", NewName: ""}
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNewNameRequired
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPatch, "/groups/rename", payload)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestGroupHandler_RenameGroup_WithNonExistentGroup_ReturnsStatus404(t *testing.T) {
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "unknown", NewName: "new name"}
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNotFound
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPatch, "/groups/rename", payload)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestGroupHandler_RenameGroup_WithExistentName_ReturnsStatus409(t *testing.T) {
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "unknown", NewName: "new name"}
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNameAlreadyExists
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPatch, "/groups/rename", payload)
	assertStatusCode(t, rec, http.StatusConflict)
}

func TestGroupHandler_RenameGroup_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, errors.New("malformed payload")
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPatch, "/groups/rename", "{}")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// RemoveGroup (DELETE /groups/{name})
// ============================================================================

func TestGroupHandler_RemoveGroup_WithValidPayload_ReturnsStatus204(t *testing.T) {
	reqUrl := fmt.Sprintf("/groups/%s", "lab1")
	mock := &mockGroupService{
		removeFn: func(_ context.Context, p schemas.RemoveGroupSchema) error {
			return nil
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodDelete, reqUrl, nil)
	assertStatusCode(t, rec, http.StatusNoContent)
}

func TestGroupHandler_RemoveGroup_WithNonExistentGroup_ReturnsStatus404(t *testing.T) {
	reqUrl := fmt.Sprintf("/groups/%s", "unknown")
	mock := &mockGroupService{
		removeFn: func(_ context.Context, p schemas.RemoveGroupSchema) error {
			return groupErrors.ErrGroupNotFound
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodDelete, reqUrl, nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

// ============================================================================
// Helpers
// ============================================================================

// setupRouter builds a router wired with a group handler backed by the
// provided service mock.
func setupRouter(t *testing.T, svc handler.GroupServiceInterface) *chi.Mux {
	t.Helper()

	r := chi.NewRouter()
	h := handler.NewGroupHandler(svc)
	h.RegisterRoutes(r)

	return r
}
