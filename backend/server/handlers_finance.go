package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"panel/finance"
	"panel/store"
)

var (
	monthRegex = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
	dateRegex  = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$`)
)

func decodeJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// ---- summary ----

func (s *Server) handleFinanceSummary(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	if at := r.URL.Query().Get("at"); at != "" {
		if t, err := time.Parse("2006-01-02", at); err == nil {
			now = t
		}
	}
	sum, err := finance.ComputeSummary(s.store, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

// ---- bills ----

func (s *Server) handleListBills(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListBills()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateBill(w http.ResponseWriter, r *http.Request) {
	var b store.Bill
	if err := decodeJSON(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateBill(&b); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.store.CreateBill(b)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateBill(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var b store.Bill
	if err := decodeJSON(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	b.ID = id
	if err := validateBill(&b); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.UpdateBill(b); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, _ := s.store.ListBills()
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleDeleteBill(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteBill(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- fixed ----

func (s *Server) handleListFixed(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListFixed()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateFixed(w http.ResponseWriter, r *http.Request) {
	var f store.FixedExpense
	if err := decodeJSON(r, &f); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateFixed(&f); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.store.CreateFixed(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateFixed(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var f store.FixedExpense
	if err := decodeJSON(r, &f); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	f.ID = id
	if err := validateFixed(&f); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.UpdateFixed(f); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, _ := s.store.ListFixed()
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleDeleteFixed(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteFixed(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- free ----

func (s *Server) handleListFree(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListFree()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateFree(w http.ResponseWriter, r *http.Request) {
	var f store.FreeExpense
	if err := decodeJSON(r, &f); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateFree(&f); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.store.CreateFree(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateFree(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var f store.FreeExpense
	if err := decodeJSON(r, &f); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	f.ID = id
	if err := validateFree(&f); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.UpdateFree(f); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, _ := s.store.ListFree()
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleDeleteFree(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteFree(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- incomes ----

func (s *Server) handleListIncomes(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListIncomes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateIncome(w http.ResponseWriter, r *http.Request) {
	var i store.Income
	if err := decodeJSON(r, &i); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateIncome(&i); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.store.CreateIncome(i)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateIncome(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var i store.Income
	if err := decodeJSON(r, &i); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	i.ID = id
	if err := validateIncome(&i); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.UpdateIncome(i); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, _ := s.store.ListIncomes()
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleDeleteIncome(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteIncome(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- settings ----

func (s *Server) handleGetFinanceSettings(w http.ResponseWriter, r *http.Request) {
	fs, err := s.store.GetFinanceSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, fs)
}

func (s *Server) handlePutFinanceSettings(w http.ResponseWriter, r *http.Request) {
	var fs store.FinanceSettings
	if err := decodeJSON(r, &fs); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if fs.AssumedNMonths <= 0 {
		fs.AssumedNMonths = 12
	}
	if fs.FreeCapMonthly < 0 || fs.InitialBalance < 0 {
		writeError(w, http.StatusBadRequest, errors.New("negative balance/cap not allowed"))
		return
	}
	if err := s.store.SetFinanceSettings(fs); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, fs)
}

// ---- validation ----

func validateBill(b *store.Bill) error {
	if strings.TrimSpace(b.Name) == "" {
		return errors.New("name empty")
	}
	if b.MonthlyAmount <= 0 {
		return errors.New("monthly_amount must be > 0")
	}
	if b.TotalMonths <= 0 {
		return errors.New("total_months must be > 0")
	}
	if !monthRegex.MatchString(b.StartedMonth) {
		return errors.New("started_month must be YYYY-MM")
	}
	return nil
}

func validateFixed(f *store.FixedExpense) error {
	if strings.TrimSpace(f.Name) == "" {
		return errors.New("name empty")
	}
	if f.MonthlyAmount <= 0 {
		return errors.New("monthly_amount must be > 0")
	}
	if !monthRegex.MatchString(f.StartedMonth) {
		return errors.New("started_month must be YYYY-MM")
	}
	return nil
}

func validateFree(f *store.FreeExpense) error {
	if !dateRegex.MatchString(f.Date) {
		return errors.New("date must be YYYY-MM-DD")
	}
	if f.Amount <= 0 {
		return errors.New("amount must be > 0")
	}
	return nil
}

func validateIncome(i *store.Income) error {
	if !dateRegex.MatchString(i.Date) {
		return errors.New("date must be YYYY-MM-DD")
	}
	if i.Amount <= 0 {
		return errors.New("amount must be > 0")
	}
	return nil
}
