package httptransport

import (
	"context"
	"errors"
	"net/http"

	"tangentry/internal/ai"
)

func (h *aiHandler) aiSettingsGet(w http.ResponseWriter, r *http.Request) {
	if !h.aiReady(w) {
		return
	}
	write(w, 200, h.ai.AI.Status())
}

func (h *aiHandler) aiSettingsPut(w http.ResponseWriter, r *http.Request) {
	if !h.aiReady(w) {
		return
	}
	var input ai.ConfigureInput
	if !decodeAIRequest(w, r, &input, 128<<10) {
		return
	}
	result, err := h.ai.AI.Configure(input)
	if err != nil {
		aiFail(w, err)
		return
	}
	write(w, 200, result)
}

func (h *aiHandler) aiModels(w http.ResponseWriter, r *http.Request) {
	if !h.aiReady(w) {
		return
	}
	var input ai.ConfigureInput
	if r.ContentLength != 0 && !decodeAIRequest(w, r, &input, 8192) {
		return
	}
	result, err := h.ai.AI.ListModels(r.Context(), input)
	if err != nil {
		aiFail(w, err)
		return
	}
	write(w, http.StatusOK, result)
}

func (h *aiHandler) aiCheck(w http.ResponseWriter, r *http.Request) {
	if !h.aiReady(w) {
		return
	}
	var input ai.CheckInput
	if !decodeAIRequest(w, r, &input, 1024) {
		return
	}
	result, err := h.ai.AI.Check(r.Context(), input)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if input.Task == "summary" {
			status, body := classifySummaryError(err)
			writeAIBudgetError(w, status, body, result.Budget, nil)
		} else {
			status, body := classifyAIError(err)
			writeAIBudgetError(w, status, body, result.Budget, nil)
		}
		return
	}
	write(w, 200, result)
}
