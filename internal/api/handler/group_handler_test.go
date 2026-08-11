package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"dougdomingos.com/aegis/internal/api/handler"
	groupErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
	"dougdomingos.com/aegis/internal/service"
)

// ============================================================================
// Service mock
// ============================================================================

type mockGroupService struct {
	createFn    func(ctx context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error)
	listFn      func(ctx context.Context) ([]schemas.GroupOutputSchema, error)
	getByNameFn func(ctx context.Context, p schemas.GetGroupByNameSchema) (*schemas.GroupOutputSchema, error)
	renameFn    func(ctx context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error)
	removeFn    func(ctx context.Context, p schemas.RemoveGroupSchema) error
}

func (m *mockGroupService) CreateGroup(ctx context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
	return m.createFn(ctx, p)
}

func (m *mockGroupService) ListAllGroups(ctx context.Context) ([]schemas.GroupOutputSchema, error) {
	return m.listFn(ctx)
}

func (m *mockGroupService) GetGroupByName(ctx context.Context, p schemas.GetGroupByNameSchema) (*schemas.GroupOutputSchema, error) {
	return m.getByNameFn(ctx, p)
}

func (m *mockGroupService) ChangeGroupName(ctx context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
	return m.renameFn(ctx, p)
}

func (m *mockGroupService) RemoveGroup(ctx context.Context, p schemas.RemoveGroupSchema) error {
	return m.removeFn(ctx, p)
}

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

	rec, req := buildTestRequest(http.MethodPost, "/groups", payload)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status code 201, go %d", rec.Code)
	}
}

func TestGroupHandler_CreateGroup_WithEmptyName_ReturnsStatus400(t *testing.T) {
	payload := schemas.CreateGroupSchema{Name: ""}
	mock := &mockGroupService{
		createFn: func(_ context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNameRequired
		},
	}

	rec, req := buildTestRequest(http.MethodPost, "/groups", payload)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
}

func TestGroupHandler_CreateGroup_WithExistentName_ReturnsStatus409(t *testing.T) {
	payload := schemas.CreateGroupSchema{Name: "lab1"}
	mock := &mockGroupService{
		createFn: func(_ context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNameAlreadyExists
		},
	}

	rec, req := buildTestRequest(http.MethodPost, "/groups", payload)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status code 409, got %d", rec.Code)
	}
}

func TestGroupHandler_CreateGroup_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockGroupService{
		createFn: func(_ context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
			return nil, errors.New("malformed payload")
		},
	}

	rec, req := buildTestRequest(http.MethodPost, "/groups", "invalid json")
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
}

func TestGroupHandler_CreateGroup_WithEmptyPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockGroupService{
		createFn: func(_ context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
			return nil, errors.New("empty payload")
		},
	}

	rec, req := buildTestRequest(http.MethodPost, "/groups", "{}")
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
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

	rec, req := buildTestRequest(http.MethodGet, reqUrl, nil)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestGroupHandler_GetGroupByName_WithNonExistentGroup_ReturnsStatus404(t *testing.T) {
	reqUrl := fmt.Sprintf("/groups/%s", "unknown")
	mock := &mockGroupService{
		getByNameFn: func(_ context.Context, p schemas.GetGroupByNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNotFound
		},
	}

	rec, req := buildTestRequest(http.MethodGet, reqUrl, nil)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status code 404, got %d", rec.Code)
	}
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

	rec, req := buildTestRequest(http.MethodGet, "/groups", nil)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestGroupHandler_ListGroups_WithoutSeededGroups_ReturnsStatus200(t *testing.T) {
	mock := &mockGroupService{
		listFn: func(_ context.Context) ([]schemas.GroupOutputSchema, error) {
			return []schemas.GroupOutputSchema{}, nil
		},
	}

	rec, req := buildTestRequest(http.MethodGet, "/groups", nil)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
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

	rec, req := buildTestRequest(http.MethodPatch, "/groups/rename", payload)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestGroupHandler_RenameGroup_WithoutTargetGroupName_ReturnsStatus400(t *testing.T) {
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "", NewName: "new name"}
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNameRequired
		},
	}

	rec, req := buildTestRequest(http.MethodPatch, "/groups/rename", payload)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
}

func TestGroupHandler_RenameGroup_WithoutNewName_ReturnsStatus400(t *testing.T) {
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "lab1", NewName: ""}
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNewNameRequired
		},
	}

	rec, req := buildTestRequest(http.MethodPatch, "/groups/rename", payload)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
}

func TestGroupHandler_RenameGroup_WithNonExistentGroup_ReturnsStatus404(t *testing.T) {
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "unknown", NewName: "new name"}
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNotFound
		},
	}

	rec, req := buildTestRequest(http.MethodPatch, "/groups/rename", payload)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status code 404, got %d", rec.Code)
	}
}

func TestGroupHandler_RenameGroup_WithExistentName_ReturnsStatus409(t *testing.T) {
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "unknown", NewName: "new name"}
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, groupErrors.ErrGroupNameAlreadyExists
		},
	}

	rec, req := buildTestRequest(http.MethodPatch, "/groups/rename", payload)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status code 409, got %d", rec.Code)
	}
}

func TestGroupHandler_RenameGroup_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockGroupService{
		renameFn: func(_ context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
			return nil, errors.New("malformed payload")
		},
	}

	rec, req := buildTestRequest(http.MethodPatch, "/groups/rename", "{}")
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
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

	rec, req := buildTestRequest(http.MethodDelete, reqUrl, nil)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status code 204, got %d", rec.Code)
	}
}

func TestGroupHandler_RemoveGroup_WithNonExistentGroup_ReturnsStatus404(t *testing.T) {
	reqUrl := fmt.Sprintf("/groups/%s", "unknown")
	mock := &mockGroupService{
		removeFn: func(_ context.Context, p schemas.RemoveGroupSchema) error {
			return groupErrors.ErrGroupNotFound
		},
	}

	rec, req := buildTestRequest(http.MethodDelete, reqUrl, nil)
	setupRouter(t, mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status code 404, got %d", rec.Code)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func setupRouter(t *testing.T, svc service.GroupServiceInterface) *chi.Mux {
	t.Helper()

	r := chi.NewRouter()
	h := handler.NewGroupHandler(svc)
	h.RegisterGroupRoutes(r)

	return r
}

func buildTestRequest(method, endpoint string, payload any) (*httptest.ResponseRecorder, *http.Request) {
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(method, endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	return httptest.NewRecorder(), req
}
