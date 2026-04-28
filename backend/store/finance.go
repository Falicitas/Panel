package store

import (
	"database/sql"
	"errors"
)

// 金额单位: 分 (int64)。日期: YYYY-MM-DD;月份: YYYY-MM。

type Bill struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	MonthlyAmount int64  `json:"monthly_amount"`
	TotalMonths   int    `json:"total_months"`
	StartedMonth  string `json:"started_month"`
}

type FixedExpense struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	MonthlyAmount int64  `json:"monthly_amount"`
	StartedMonth  string `json:"started_month"`
}

type FreeExpense struct {
	ID     int64  `json:"id"`
	Date   string `json:"date"`
	Amount int64  `json:"amount"`
	Note   string `json:"note"`
}

type Income struct {
	ID     int64  `json:"id"`
	Date   string `json:"date"`
	Amount int64  `json:"amount"`
	Note   string `json:"note"`
}

type FinanceSettings struct {
	InitialBalance    int64   `json:"initial_balance"`
	FreeCapMonthly    int64   `json:"free_cap_monthly"`
	AssumedNMonths    int     `json:"assumed_n_months"`
	AssumedAnnualRate float64 `json:"assumed_annual_rate"`
}

// ---- bills ----

func (s *Store) ListBills() ([]Bill, error) {
	rows, err := s.db.Query(`SELECT id, name, monthly_amount, total_months, started_month FROM bills ORDER BY started_month, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Bill{}
	for rows.Next() {
		var b Bill
		if err := rows.Scan(&b.ID, &b.Name, &b.MonthlyAmount, &b.TotalMonths, &b.StartedMonth); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) CreateBill(b Bill) (Bill, error) {
	res, err := s.db.Exec(
		`INSERT INTO bills (name, monthly_amount, total_months, started_month) VALUES (?, ?, ?, ?)`,
		b.Name, b.MonthlyAmount, b.TotalMonths, b.StartedMonth,
	)
	if err != nil {
		return Bill{}, err
	}
	id, _ := res.LastInsertId()
	b.ID = id
	return b, nil
}

func (s *Store) UpdateBill(b Bill) error {
	_, err := s.db.Exec(
		`UPDATE bills SET name=?, monthly_amount=?, total_months=?, started_month=? WHERE id=?`,
		b.Name, b.MonthlyAmount, b.TotalMonths, b.StartedMonth, b.ID,
	)
	return err
}

func (s *Store) DeleteBill(id int64) error {
	_, err := s.db.Exec(`DELETE FROM bills WHERE id=?`, id)
	return err
}

// ---- fixed_expenses ----

func (s *Store) ListFixed() ([]FixedExpense, error) {
	rows, err := s.db.Query(`SELECT id, name, monthly_amount, started_month FROM fixed_expenses ORDER BY started_month, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FixedExpense{}
	for rows.Next() {
		var f FixedExpense
		if err := rows.Scan(&f.ID, &f.Name, &f.MonthlyAmount, &f.StartedMonth); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) CreateFixed(f FixedExpense) (FixedExpense, error) {
	res, err := s.db.Exec(
		`INSERT INTO fixed_expenses (name, monthly_amount, started_month) VALUES (?, ?, ?)`,
		f.Name, f.MonthlyAmount, f.StartedMonth,
	)
	if err != nil {
		return FixedExpense{}, err
	}
	id, _ := res.LastInsertId()
	f.ID = id
	return f, nil
}

func (s *Store) UpdateFixed(f FixedExpense) error {
	_, err := s.db.Exec(
		`UPDATE fixed_expenses SET name=?, monthly_amount=?, started_month=? WHERE id=?`,
		f.Name, f.MonthlyAmount, f.StartedMonth, f.ID,
	)
	return err
}

func (s *Store) DeleteFixed(id int64) error {
	_, err := s.db.Exec(`DELETE FROM fixed_expenses WHERE id=?`, id)
	return err
}

// ---- free_expenses ----

func (s *Store) ListFree() ([]FreeExpense, error) {
	rows, err := s.db.Query(`SELECT id, date, amount, note FROM free_expenses ORDER BY date DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FreeExpense{}
	for rows.Next() {
		var f FreeExpense
		if err := rows.Scan(&f.ID, &f.Date, &f.Amount, &f.Note); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) CreateFree(f FreeExpense) (FreeExpense, error) {
	res, err := s.db.Exec(
		`INSERT INTO free_expenses (date, amount, note) VALUES (?, ?, ?)`,
		f.Date, f.Amount, f.Note,
	)
	if err != nil {
		return FreeExpense{}, err
	}
	id, _ := res.LastInsertId()
	f.ID = id
	return f, nil
}

func (s *Store) UpdateFree(f FreeExpense) error {
	_, err := s.db.Exec(
		`UPDATE free_expenses SET date=?, amount=?, note=? WHERE id=?`,
		f.Date, f.Amount, f.Note, f.ID,
	)
	return err
}

func (s *Store) DeleteFree(id int64) error {
	_, err := s.db.Exec(`DELETE FROM free_expenses WHERE id=?`, id)
	return err
}

// ---- incomes ----

func (s *Store) ListIncomes() ([]Income, error) {
	rows, err := s.db.Query(`SELECT id, date, amount, note FROM incomes ORDER BY date DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Income{}
	for rows.Next() {
		var i Income
		if err := rows.Scan(&i.ID, &i.Date, &i.Amount, &i.Note); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) CreateIncome(i Income) (Income, error) {
	res, err := s.db.Exec(
		`INSERT INTO incomes (date, amount, note) VALUES (?, ?, ?)`,
		i.Date, i.Amount, i.Note,
	)
	if err != nil {
		return Income{}, err
	}
	id, _ := res.LastInsertId()
	i.ID = id
	return i, nil
}

func (s *Store) UpdateIncome(i Income) error {
	_, err := s.db.Exec(
		`UPDATE incomes SET date=?, amount=?, note=? WHERE id=?`,
		i.Date, i.Amount, i.Note, i.ID,
	)
	return err
}

func (s *Store) DeleteIncome(id int64) error {
	_, err := s.db.Exec(`DELETE FROM incomes WHERE id=?`, id)
	return err
}

// ---- finance_settings (single row) ----

func (s *Store) GetFinanceSettings() (FinanceSettings, error) {
	var fs FinanceSettings
	err := s.db.QueryRow(
		`SELECT initial_balance, free_cap_monthly, assumed_n_months, assumed_annual_rate FROM finance_settings WHERE id=1`,
	).Scan(&fs.InitialBalance, &fs.FreeCapMonthly, &fs.AssumedNMonths, &fs.AssumedAnnualRate)
	if errors.Is(err, sql.ErrNoRows) {
		return FinanceSettings{InitialBalance: 0, FreeCapMonthly: 100000, AssumedNMonths: 12, AssumedAnnualRate: 0.12}, nil
	}
	return fs, err
}

func (s *Store) SetFinanceSettings(fs FinanceSettings) error {
	_, err := s.db.Exec(
		`UPDATE finance_settings SET initial_balance=?, free_cap_monthly=?, assumed_n_months=?, assumed_annual_rate=? WHERE id=1`,
		fs.InitialBalance, fs.FreeCapMonthly, fs.AssumedNMonths, fs.AssumedAnnualRate,
	)
	return err
}
