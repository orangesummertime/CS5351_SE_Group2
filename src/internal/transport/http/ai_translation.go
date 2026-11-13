package httptransport

import (
	"context"
	"errors"
	"net/http"

	"tangentry/internal/ai"
)

func (h *aiHandler) aiSubtitleInspect(w http.ResponseWriter, r *http.Request) {
	if !h.aiReady(w) {
		return
	}
	var input ai.Input
	if !decodeAIRequest(w, r, &input, ai.MaxInputBytes) {
		return
	}
	result, err := h.ai.AI.Inspect(r.Context(), input)
	if err != nil {
		aiFail(w, err)
		return
	}
	write(w, 200, result)
}

func (h *aiHandler) aiSubtitleTranslate(w http.ResponseWriter, r *http.Request) {
	if !h.aiReady(w) {
		return
	}
	var input ai.Input
	if !decodeAIRequest(w, r, &input, ai.MaxInputBytes) {
		return
	}
	// Translation may span many small batches, so allow a bounded long write.
	if err := setAILongWriteDeadline(w); err != nil && !errors.Is(err, http.ErrNotSupported) {
		aiFail(w, ai.ErrProvider)
		return
	}
	result, err := h.ai.AI.Translate(r.Context(), input)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		status, body := classifyAIError(err)
		writeAIBudgetError(w, status, body, result.Budget, translationPartial(result))
		return
	}
	write(w, 200, result)
}

func (h *aiHandler) aiSubtitleCached(w http.ResponseWriter, r *http.Request) {
	if !h.aiReady(w) {
		return
	}
	if len(r.URL.Query()) != 0 {
		aiFail(w, ai.ErrInvalid)
		return
	}
	result, err := h.ai.AI.Cached(r.Context(), r.PathValue("key"))
	if err != nil {
		aiFail(w, err)
		return
	}
	write(w, 200, result)
}
