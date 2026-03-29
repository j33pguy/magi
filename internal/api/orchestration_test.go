package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

func TestHandleRegisterAgent(t *testing.T) {
	s := newTestServer(t)
	body := `{"id":"grok-1","name":"Grok","capabilities":["research","x-scraping"],"endpoint":"http://localhost:9000"}`
	req := httptest.NewRequest("POST", "/agents", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRegisterAgent(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var agent db.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	if agent.ID != "grok-1" {
		t.Errorf("expected grok-1, got %s", agent.ID)
	}
	if agent.Status != "online" {
		t.Errorf("expected online, got %s", agent.Status)
	}
}

func TestHandleRegisterAgent_MissingName(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("POST", "/agents", strings.NewReader(`{"id":"x"}`))
	w := httptest.NewRecorder()
	s.handleRegisterAgent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleListAgents(t *testing.T) {
	s := newTestServer(t)
	s.db.RegisterAgent(&db.Agent{ID: "a1", Name: "Alpha", Capabilities: []string{"code"}})
	s.db.RegisterAgent(&db.Agent{ID: "a2", Name: "Beta", Capabilities: []string{"research"}})

	req := httptest.NewRequest("GET", "/agents", nil)
	w := httptest.NewRecorder()
	s.handleListAgents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var agents []db.Agent
	json.NewDecoder(w.Body).Decode(&agents)
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestHandleListAgents_Empty(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/agents", nil)
	w := httptest.NewRecorder()
	s.handleListAgents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("expected empty array, got %s", w.Body.String())
	}
}

func TestHandleGetAgent(t *testing.T) {
	s := newTestServer(t)
	s.db.RegisterAgent(&db.Agent{ID: "g1", Name: "Grok"})

	req := httptest.NewRequest("GET", "/agents/g1", nil)
	req.SetPathValue("id", "g1")
	w := httptest.NewRecorder()
	s.handleGetAgent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var agent db.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	if agent.Name != "Grok" {
		t.Errorf("expected Grok, got %s", agent.Name)
	}
}

func TestHandleGetAgent_NotFound(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/agents/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	s.handleGetAgent(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleHeartbeatAgent(t *testing.T) {
	s := newTestServer(t)
	s.db.RegisterAgent(&db.Agent{ID: "h1", Name: "Heartbeat"})

	req := httptest.NewRequest("POST", "/agents/h1/heartbeat", nil)
	req.SetPathValue("id", "h1")
	w := httptest.NewRecorder()
	s.handleHeartbeatAgent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleDeregisterAgent(t *testing.T) {
	s := newTestServer(t)
	s.db.RegisterAgent(&db.Agent{ID: "d1", Name: "Delete"})

	req := httptest.NewRequest("DELETE", "/agents/d1", nil)
	req.SetPathValue("id", "d1")
	w := httptest.NewRecorder()
	s.handleDeregisterAgent(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	req2 := httptest.NewRequest("GET", "/agents/d1", nil)
	req2.SetPathValue("id", "d1")
	w2 := httptest.NewRecorder()
	s.handleGetAgent(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w2.Code)
	}
}

func TestHandleCreateTask(t *testing.T) {
	s := newTestServer(t)
	body := `{"description":"Build adapter","created_by":"orchestrator","subtasks":[{"description":"Research API"},{"description":"Design"},{"description":"Implement"}]}`
	req := httptest.NewRequest("POST", "/tasks", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleCreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var task db.Task
	json.NewDecoder(w.Body).Decode(&task)
	if task.Status != "created" {
		t.Errorf("expected created, got %s", task.Status)
	}
	if len(task.Subtasks) != 3 {
		t.Errorf("expected 3 subtasks, got %d", len(task.Subtasks))
	}
}

func TestHandleCreateTask_MissingDescription(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("POST", "/tasks", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.handleCreateTask(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleGetTask(t *testing.T) {
	s := newTestServer(t)
	task, _ := s.db.CreateTask(&db.Task{
		Description: "Test task",
		Subtasks:    []*db.Subtask{{Description: "Sub 1"}, {Description: "Sub 2"}},
	})

	req := httptest.NewRequest("GET", "/tasks/"+task.ID, nil)
	req.SetPathValue("id", task.ID)
	w := httptest.NewRecorder()
	s.handleGetTask(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result db.Task
	json.NewDecoder(w.Body).Decode(&result)
	if len(result.Subtasks) != 2 {
		t.Errorf("expected 2 subtasks, got %d", len(result.Subtasks))
	}
}

func TestHandleGetTask_NotFound(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/tasks/nope", nil)
	req.SetPathValue("id", "nope")
	w := httptest.NewRecorder()
	s.handleGetTask(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleListTasks(t *testing.T) {
	s := newTestServer(t)
	s.db.CreateTask(&db.Task{Description: "Task 1"})
	s.db.CreateTask(&db.Task{Description: "Task 2"})

	req := httptest.NewRequest("GET", "/tasks", nil)
	w := httptest.NewRecorder()
	s.handleListTasks(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var tasks []db.Task
	json.NewDecoder(w.Body).Decode(&tasks)
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestHandleListTasks_Empty(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/tasks", nil)
	w := httptest.NewRecorder()
	s.handleListTasks(w, req)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("expected empty array, got %s", w.Body.String())
	}
}

func TestHandleReportProgress(t *testing.T) {
	s := newTestServer(t)
	task, _ := s.db.CreateTask(&db.Task{
		Description: "Progress test",
		Subtasks:    []*db.Subtask{{Description: "Work"}},
	})

	body := `{"percent":75,"message":"Almost there"}`
	req := httptest.NewRequest("POST", "/tasks/subtasks/"+task.Subtasks[0].ID+"/progress", strings.NewReader(body))
	req.SetPathValue("subtask_id", task.Subtasks[0].ID)
	w := httptest.NewRecorder()
	s.handleReportProgress(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, _ := s.db.GetTask(task.ID)
	if updated.Subtasks[0].ProgressPercent != 75 {
		t.Errorf("expected 75%%, got %d%%", updated.Subtasks[0].ProgressPercent)
	}
}

func TestHandleUpdateSubtask_AssignAndComplete(t *testing.T) {
	s := newTestServer(t)
	s.db.RegisterAgent(&db.Agent{ID: "grok", Name: "Grok"})
	task, _ := s.db.CreateTask(&db.Task{
		Description: "Update test",
		Subtasks:    []*db.Subtask{{Description: "Research"}},
	})
	subID := task.Subtasks[0].ID

	body := `{"agent_id":"grok"}`
	req := httptest.NewRequest("PATCH", "/tasks/subtasks/"+subID, strings.NewReader(body))
	req.SetPathValue("subtask_id", subID)
	w := httptest.NewRecorder()
	s.handleUpdateSubtask(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("assign: expected 200, got %d", w.Code)
	}

	body2 := `{"status":"complete","output":"Found 3 issues"}`
	req2 := httptest.NewRequest("PATCH", "/tasks/subtasks/"+subID, strings.NewReader(body2))
	req2.SetPathValue("subtask_id", subID)
	w2 := httptest.NewRecorder()
	s.handleUpdateSubtask(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("complete: expected 200, got %d", w2.Code)
	}

	updated, _ := s.db.GetTask(task.ID)
	if updated.Subtasks[0].Status != "complete" {
		t.Errorf("expected complete, got %s", updated.Subtasks[0].Status)
	}
	if updated.Subtasks[0].Output != "Found 3 issues" {
		t.Errorf("expected output, got %s", updated.Subtasks[0].Output)
	}
}

func TestOrchestrationE2E(t *testing.T) {
	s := newTestServer(t)

	// Register 3 agents
	for _, a := range []struct{ id, name string }{{"grok", "Grok"}, {"claude", "Claude"}, {"codex", "Codex"}} {
		body := `{"id":"` + a.id + `","name":"` + a.name + `"}`
		req := httptest.NewRequest("POST", "/agents", strings.NewReader(body))
		w := httptest.NewRecorder()
		s.handleRegisterAgent(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("register %s: %d", a.id, w.Code)
		}
	}

	// Create task with 3 subtasks
	taskBody := `{"description":"API migration","created_by":"orchestrator","subtasks":[{"description":"Research breaking changes"},{"description":"Design adapter pattern"},{"description":"Implement adapter"}]}`
	req := httptest.NewRequest("POST", "/tasks", strings.NewReader(taskBody))
	w := httptest.NewRecorder()
	s.handleCreateTask(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create task: %d", w.Code)
	}
	var task db.Task
	json.NewDecoder(w.Body).Decode(&task)

	// Assign subtasks to agents
	agents := []string{"grok", "claude", "codex"}
	for i, agentID := range agents {
		body := `{"agent_id":"` + agentID + `"}`
		req := httptest.NewRequest("PATCH", "/tasks/subtasks/"+task.Subtasks[i].ID, strings.NewReader(body))
		req.SetPathValue("subtask_id", task.Subtasks[i].ID)
		w := httptest.NewRecorder()
		s.handleUpdateSubtask(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("assign %s: %d", agentID, w.Code)
		}
	}

	// Progress + complete each subtask
	for i, sub := range task.Subtasks {
		pBody := `{"percent":50,"message":"halfway"}`
		pReq := httptest.NewRequest("POST", "/tasks/subtasks/"+sub.ID+"/progress", strings.NewReader(pBody))
		pReq.SetPathValue("subtask_id", sub.ID)
		pW := httptest.NewRecorder()
		s.handleReportProgress(pW, pReq)

		cBody := `{"status":"complete","output":"done"}`
		cReq := httptest.NewRequest("PATCH", "/tasks/subtasks/"+sub.ID, strings.NewReader(cBody))
		cReq.SetPathValue("subtask_id", sub.ID)
		cW := httptest.NewRecorder()
		s.handleUpdateSubtask(cW, cReq)
		if cW.Code != http.StatusOK {
			t.Fatalf("complete subtask %d: %d", i, cW.Code)
		}
	}

	// Verify final task state
	fReq := httptest.NewRequest("GET", "/tasks/"+task.ID, nil)
	fReq.SetPathValue("id", task.ID)
	fW := httptest.NewRecorder()
	s.handleGetTask(fW, fReq)

	var final db.Task
	json.NewDecoder(fW.Body).Decode(&final)
	for _, sub := range final.Subtasks {
		if sub.Status != "complete" {
			t.Errorf("subtask %s not complete: %s", sub.Description, sub.Status)
		}
	}

	// Verify all 3 agents still registered
	aReq := httptest.NewRequest("GET", "/agents", nil)
	aW := httptest.NewRecorder()
	s.handleListAgents(aW, aReq)
	var allAgents []db.Agent
	json.NewDecoder(aW.Body).Decode(&allAgents)
	if len(allAgents) != 3 {
		t.Errorf("expected 3 agents, got %d", len(allAgents))
	}
}
