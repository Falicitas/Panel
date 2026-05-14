package server

import (
	"log"
	"net/http"

	"panel/micro"
	"panel/store"
	"panel/todolist"
)

type Server struct {
	repo  *todolist.Repo
	store *store.Store
	micro *micro.Repo
}

func New(repo *todolist.Repo, st *store.Store, microRepo *micro.Repo) *Server {
	return &Server{repo: repo, store: st, micro: microRepo}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/todolists", s.handleListTodolists)
	mux.HandleFunc("POST /api/todolists", s.handleCreateTodolist)
	mux.HandleFunc("GET /api/todolists/{name}", s.handleGetTodolist)
	mux.HandleFunc("PUT /api/todolists/{name}", s.handlePutTodolist)
	mux.HandleFunc("GET /api/anchor", s.handleGetAnchor)
	mux.HandleFunc("POST /api/anchor", s.handlePostAnchor)
	mux.HandleFunc("GET /api/focus", s.handleGetFocus)
	mux.HandleFunc("GET /api/micro/today", s.handleGetMicroToday)
	mux.HandleFunc("PUT /api/micro/today", s.handlePutMicroToday)
	mux.HandleFunc("GET /api/micro/today/progress", s.handleGetMicroProgress)
	mux.HandleFunc("GET /api/recurring", s.handleListRecurring)
	mux.HandleFunc("POST /api/recurring", s.handleCreateRecurring)
	mux.HandleFunc("POST /api/recurring/reorder", s.handleReorderRecurring)
	mux.HandleFunc("PUT /api/recurring/{id}", s.handleUpdateRecurring)
	mux.HandleFunc("DELETE /api/recurring/{id}", s.handleDeleteRecurring)
	mux.HandleFunc("GET /api/finance/summary", s.handleFinanceSummary)
	mux.HandleFunc("GET /api/finance/bills", s.handleListBills)
	mux.HandleFunc("POST /api/finance/bills", s.handleCreateBill)
	mux.HandleFunc("PUT /api/finance/bills/{id}", s.handleUpdateBill)
	mux.HandleFunc("DELETE /api/finance/bills/{id}", s.handleDeleteBill)
	mux.HandleFunc("GET /api/finance/fixed", s.handleListFixed)
	mux.HandleFunc("POST /api/finance/fixed", s.handleCreateFixed)
	mux.HandleFunc("PUT /api/finance/fixed/{id}", s.handleUpdateFixed)
	mux.HandleFunc("DELETE /api/finance/fixed/{id}", s.handleDeleteFixed)
	mux.HandleFunc("GET /api/finance/free", s.handleListFree)
	mux.HandleFunc("POST /api/finance/free", s.handleCreateFree)
	mux.HandleFunc("PUT /api/finance/free/{id}", s.handleUpdateFree)
	mux.HandleFunc("DELETE /api/finance/free/{id}", s.handleDeleteFree)
	mux.HandleFunc("GET /api/finance/incomes", s.handleListIncomes)
	mux.HandleFunc("POST /api/finance/incomes", s.handleCreateIncome)
	mux.HandleFunc("PUT /api/finance/incomes/{id}", s.handleUpdateIncome)
	mux.HandleFunc("DELETE /api/finance/incomes/{id}", s.handleDeleteIncome)
	mux.HandleFunc("GET /api/finance/settings", s.handleGetFinanceSettings)
	mux.HandleFunc("PUT /api/finance/settings", s.handlePutFinanceSettings)
	mux.HandleFunc("GET /api/supplements", s.handleListSupplements)
	mux.HandleFunc("GET /api/supplements/today", s.handleGetSupplementsToday)
	mux.HandleFunc("POST /api/supplements", s.handleCreateSupplement)
	mux.HandleFunc("PUT /api/supplements/{id}", s.handleUpdateSupplement)
	mux.HandleFunc("DELETE /api/supplements/{id}", s.handleDeleteSupplement)
	mux.HandleFunc("POST /api/supplements/{id}/check", s.handleCheckSupplement)
	mux.HandleFunc("POST /api/supplements/{id}/pause", s.handlePauseSupplement)
	mux.HandleFunc("POST /api/supplements/{id}/enable", s.handleEnableSupplement)
	mux.HandleFunc("POST /api/supplements/{id}/snooze", s.handleSnoozeSupplement)
	mux.HandleFunc("POST /api/supplements/reorder", s.handleReorderSupplements)
	mux.HandleFunc("GET /api/state/today", s.handleGetTodayState)
	mux.HandleFunc("POST /api/state/today", s.handlePostTodayState)
	mux.HandleFunc("GET /api/state/settings", s.handleGetStateSettings)
	mux.HandleFunc("PUT /api/state/settings", s.handlePutStateSettings)
	mux.HandleFunc("POST /api/state/meal", s.handlePostMeal)
	mux.HandleFunc("POST /api/state/meal/{meal}/pause", s.handlePauseMeal)
	mux.HandleFunc("POST /api/state/meal/{meal}/enable", s.handleEnableMeal)
	mux.HandleFunc("GET /api/notifications", s.handleListNotifications)
	mux.HandleFunc("POST /api/notifications/{id}/dismiss", s.handleDismissNotification)
	mux.HandleFunc("POST /api/records/grip", s.handlePostGrip)
	mux.HandleFunc("GET /api/records/grip/today", s.handleGetGripToday)
	mux.HandleFunc("GET /api/records/supplements/month", s.handleGetSupplementsMonth)
	mux.HandleFunc("POST /api/exec", s.handlePostExec)
	mux.HandleFunc("GET /api/exec/history", s.handleListBashHistory)
	mux.HandleFunc("GET /api/english/today", s.handleEnglishToday)
	mux.HandleFunc("GET /api/english/audios", s.handleListEnglishAudios)
	mux.HandleFunc("GET /api/english/audios/candidate", s.handleEnglishCandidate)
	mux.HandleFunc("GET /api/english/audios/{id}", s.handleGetEnglishAudio)
	mux.HandleFunc("DELETE /api/english/audios/{id}", s.handleDeleteEnglishAudio)
	mux.HandleFunc("GET /api/english/audios/{id}/file", s.handleGetEnglishAudioFile)
	mux.HandleFunc("POST /api/english/audios/{id}/skip", s.handleSkipEnglishAudio)
	mux.HandleFunc("PUT /api/english/audios/{id}/segments", s.handlePutEnglishSegments)
	mux.HandleFunc("POST /api/english/segments/check", s.handleCheckEnglishSegment)
	mux.HandleFunc("POST /api/english/windows/{idx}/snooze", s.handleSnoozeEnglishWindow)
	mux.HandleFunc("GET /api/english/settings", s.handleGetEnglishSettings)
	mux.HandleFunc("PUT /api/english/settings", s.handlePutEnglishSettings)
	mux.HandleFunc("GET /api/english/review/today", s.handleEnglishReviewToday)
	mux.HandleFunc("POST /api/english/review/{id}/done", s.handleReviewDone)
	mux.HandleFunc("POST /api/english/review/{id}/postpone", s.handleReviewPostpone)
	mux.HandleFunc("POST /api/english/review/{id}/finish", s.handleReviewFinish)
	return logMiddleware(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func logMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		h.ServeHTTP(w, r)
	})
}
