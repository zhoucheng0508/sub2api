package handler

import (
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMediaVideoConflictClassification(t *testing.T) {
	for _, test := range []struct {
		err      error
		conflict bool
	}{
		{service.ErrMediaVideoIdempotencyConflict, true},
		{errors.New("upstream status 409"), false},
	} {
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		new(MediaVideoHandler).createError(ctx, http.StatusConflict, test.err)
		if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), "VIDEO_IDEMPOTENCY_CONFLICT") != test.conflict {
			t.Fatalf("invalid conflict classification: %s", response.Body.String())
		}
	}
}

func TestMediaVideoSubmissionStateContract(t *testing.T) {
	future := time.Now().Add(time.Hour)
	for _, state := range []string{"prepared", "submitting", "uncertain", "accepted", "rejected"} {
		task := &service.MediaVideoTask{TaskID: "test", Status: "creating", SubmissionState: state, ExpiresAt: &future}
		payload := taskJSON(task)
		if payload["submission_state"] != state || payload["can_continue"] != (state == "prepared") || payload["reconciliation_required"] != (state == "uncertain") {
			t.Fatalf("invalid task contract: %v", payload)
		}
		task.UpstreamTaskID = "already-submitted"
		if mediaVideoCanContinue(task) {
			t.Fatal("mapped order cannot be continued")
		}
	}
	past := time.Now().Add(-time.Hour)
	if mediaVideoCanContinue(&service.MediaVideoTask{Status: "creating", SubmissionState: "prepared", ExpiresAt: &past}) {
		t.Fatal("expired order cannot be continued")
	}
}
