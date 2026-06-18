package aisite

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

const (
	PhaseAnalyze = "analyze"
	PhasePlan    = "plan"
	PhaseExecute = "execute"
	PhaseDeploy  = "deploy"
)

const (
	StepPending = "pending"
	StepRunning = "running"
	StepDone    = "done"
	StepFailed  = "failed"
)

var pipelinePhaseOrder = []string{PhaseAnalyze, PhasePlan, PhaseExecute, PhaseDeploy}

type PipelineStep struct {
	Phase     string     `json:"phase"`
	Status    string     `json:"status"`
	Log       string     `json:"log"`
	Error     string     `json:"error,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type pipelineTracker struct {
	jobID   uint
	service *Service
	steps   []PipelineStep
}

func newPipelineSteps() []PipelineStep {
	steps := make([]PipelineStep, len(pipelinePhaseOrder))
	for i, phase := range pipelinePhaseOrder {
		steps[i] = PipelineStep{Phase: phase, Status: StepPending}
	}
	return steps
}

func parsePipelineSteps(raw string) []PipelineStep {
	if strings.TrimSpace(raw) == "" {
		return newPipelineSteps()
	}
	var steps []PipelineStep
	if err := json.Unmarshal([]byte(raw), &steps); err != nil || len(steps) == 0 {
		return newPipelineSteps()
	}
	return steps
}

func (t *pipelineTracker) startPhase(phase string) {
	now := time.Now()
	for i := range t.steps {
		if t.steps[i].Phase == phase {
			t.steps[i].Status = StepRunning
			t.steps[i].StartedAt = &now
			t.steps[i].EndedAt = nil
			t.steps[i].Error = ""
			break
		}
	}
	t.persist(phase)
}

func (t *pipelineTracker) appendLog(phase, msg string) {
	line := fmt.Sprintf("[%s] %s", ts(), msg)
	for i := range t.steps {
		if t.steps[i].Phase == phase {
			if t.steps[i].Log != "" {
				t.steps[i].Log += "\n"
			}
			t.steps[i].Log += line
			break
		}
	}
	t.persist(phase)
}

func (t *pipelineTracker) finishPhase(phase, status, errMsg string) {
	now := time.Now()
	for i := range t.steps {
		if t.steps[i].Phase == phase {
			t.steps[i].Status = status
			t.steps[i].EndedAt = &now
			t.steps[i].Error = errMsg
			break
		}
	}
	t.persist(phase)
}

func (t *pipelineTracker) phaseLog(phase string) func(string) {
	return func(msg string) {
		t.appendLog(phase, msg)
	}
}

func (t *pipelineTracker) runningPhase() string {
	for i := range t.steps {
		if t.steps[i].Status == StepRunning {
			return t.steps[i].Phase
		}
	}
	return PhasePlan
}

func (t *pipelineTracker) combinedLog() string {
	var parts []string
	for _, step := range t.steps {
		if step.Log == "" {
			continue
		}
		parts = append(parts, step.Log)
	}
	return strings.Join(parts, "\n")
}

func (t *pipelineTracker) persist(currentPhase string) {
	stepsJSON, _ := json.Marshal(t.steps)
	updates := map[string]interface{}{
		"steps_json":    string(stepsJSON),
		"current_phase": currentPhase,
		"log":           t.combinedLog(),
	}
	_ = t.service.db.Model(&models.AISiteBootstrapJob{}).Where("id = ?", t.jobID).Updates(updates).Error
}

func stepsToJSON(steps []PipelineStep) string {
	b, _ := json.Marshal(steps)
	return string(b)
}
