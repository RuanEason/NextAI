package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"copaw-next/apps/gateway/internal/config"
	"copaw-next/apps/gateway/internal/domain"
	"copaw-next/apps/gateway/internal/repo"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir, err := os.MkdirTemp("", "copaw-next-gateway-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	srv, err := NewServer(config.Config{Host: "127.0.0.1", Port: "0", DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}

func TestChatCreateAndGetHistory(t *testing.T) {
	srv := newTestServer(t)

	createReq := `{"name":"A","session_id":"s1","user_id":"u1","channel":"console","meta":{}}`
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, httptest.NewRequest(http.MethodPost, "/chats", strings.NewReader(createReq)))
	if w1.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", w1.Code, w1.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(w1.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	chatID, _ := created["id"].(string)
	if chatID == "" {
		t.Fatalf("empty chat id")
	}

	procReq := `{"input":[{"role":"user","type":"message","content":[{"type":"text","text":"hello"}]}],"session_id":"s1","user_id":"u1","channel":"console","stream":false}`
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/agent/process", strings.NewReader(procReq)))
	if w2.Code != http.StatusOK {
		t.Fatalf("process status=%d body=%s", w2.Code, w2.Body.String())
	}

	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/chats/"+chatID, nil))
	if w3.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", w3.Code, w3.Body.String())
	}
	if !strings.Contains(w3.Body.String(), "assistant") {
		t.Fatalf("history should contain assistant message: %s", w3.Body.String())
	}
}

func TestWorkspaceUploadRejectUnsafePath(t *testing.T) {
	srv := newTestServer(t)

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	f, err := zw.Create("../evil.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("x"))
	_ = zw.Close()

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	part, err := mw.CreateFormFile("file", "workspace.zip")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(part, bytes.NewReader(buf.Bytes()))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/workspace/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestProcessAgentOpenAIRequiresAPIKey(t *testing.T) {
	srv := newTestServer(t)

	setActive := `{"provider_id":"openai","model":"gpt-4o-mini"}`
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, httptest.NewRequest(http.MethodPut, "/models/active", strings.NewReader(setActive)))
	if w1.Code != http.StatusOK {
		t.Fatalf("set active status=%d body=%s", w1.Code, w1.Body.String())
	}

	procReq := `{"input":[{"role":"user","type":"message","content":[{"type":"text","text":"hello"}]}],"session_id":"s1","user_id":"u1","channel":"console","stream":false}`
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/agent/process", strings.NewReader(procReq)))
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), `"code":"provider_not_configured"`) {
		t.Fatalf("unexpected error body: %s", w2.Body.String())
	}
}

func TestProcessAgentOpenAIConfigured(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"provider reply"}}]}`))
	}))
	defer mock.Close()

	srv := newTestServer(t)

	configProvider := `{"api_key":"sk-test","base_url":"` + mock.URL + `"}`
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, httptest.NewRequest(http.MethodPut, "/models/openai/config", strings.NewReader(configProvider)))
	if w1.Code != http.StatusOK {
		t.Fatalf("config provider status=%d body=%s", w1.Code, w1.Body.String())
	}

	setActive := `{"provider_id":"openai","model":"gpt-4o-mini"}`
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, httptest.NewRequest(http.MethodPut, "/models/active", strings.NewReader(setActive)))
	if w2.Code != http.StatusOK {
		t.Fatalf("set active status=%d body=%s", w2.Code, w2.Body.String())
	}

	procReq := `{"input":[{"role":"user","type":"message","content":[{"type":"text","text":"hello"}]}],"session_id":"s1","user_id":"u1","channel":"console","stream":false}`
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, httptest.NewRequest(http.MethodPost, "/agent/process", strings.NewReader(procReq)))
	if w3.Code != http.StatusOK {
		t.Fatalf("process status=%d body=%s", w3.Code, w3.Body.String())
	}
	if !strings.Contains(w3.Body.String(), `"provider reply"`) {
		t.Fatalf("unexpected body: %s", w3.Body.String())
	}
}

func TestCronSchedulerRunsIntervalJob(t *testing.T) {
	srv := newTestServer(t)
	createReq := `{
		"id":"job-interval",
		"name":"job-interval",
		"enabled":true,
		"schedule":{"type":"interval","cron":"1s"},
		"task_type":"text",
		"text":"hello cron",
		"dispatch":{"target":{"user_id":"u1","session_id":"s1"}}
	}`
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/cron/jobs", strings.NewReader(createReq)))
	if w.Code != http.StatusOK {
		t.Fatalf("create cron status=%d body=%s", w.Code, w.Body.String())
	}

	state := waitForCronState(t, srv, "job-interval", 5*time.Second, func(v map[string]interface{}) bool {
		_, ok := v["last_run_at"].(string)
		return ok
	})
	if got, _ := state["last_status"].(string); got != cronStatusSucceeded {
		t.Fatalf("expected last_status=%q, got=%v", cronStatusSucceeded, state["last_status"])
	}
	if _, ok := state["next_run_at"].(string); !ok {
		t.Fatalf("expected next_run_at to be set: %+v", state)
	}
}

func TestCronSchedulerRecoversPersistedDueJob(t *testing.T) {
	dir, err := os.MkdirTemp("", "copaw-next-gateway-recovery-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	store, err := repo.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-2 * time.Second).Format(time.RFC3339)
	if err := store.Write(func(state *repo.State) error {
		state.CronJobs["job-recover"] = domain.CronJobSpec{
			ID:      "job-recover",
			Name:    "job-recover",
			Enabled: true,
			Schedule: domain.CronScheduleSpec{
				Type: "interval",
				Cron: "1s",
			},
			TaskType: "text",
			Text:     "recover",
			Dispatch: domain.CronDispatchSpec{
				Target: domain.CronDispatchTarget{
					UserID:    "u1",
					SessionID: "s1",
				},
			},
		}
		state.CronStates["job-recover"] = domain.CronJobState{NextRunAt: &past}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(config.Config{Host: "127.0.0.1", Port: "0", DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })

	state := waitForCronState(t, srv, "job-recover", 5*time.Second, func(v map[string]interface{}) bool {
		_, ok := v["last_run_at"].(string)
		return ok
	})
	if got, _ := state["last_status"].(string); got != cronStatusSucceeded {
		t.Fatalf("expected last_status=%q, got=%v", cronStatusSucceeded, state["last_status"])
	}
}

func waitForCronState(t *testing.T, srv *Server, jobID string, timeout time.Duration, pred func(v map[string]interface{}) bool) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last map[string]interface{}
	for time.Now().Before(deadline) {
		last = getCronState(t, srv, jobID)
		if pred(last) {
			return last
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting cron state for %s: %+v", jobID, last)
	return nil
}

func getCronState(t *testing.T, srv *Server, jobID string) map[string]interface{} {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cron/jobs/"+jobID+"/state", nil)
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get cron state status=%d body=%s", w.Code, w.Body.String())
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode cron state failed: %v body=%s", err, w.Body.String())
	}
	return out
}
