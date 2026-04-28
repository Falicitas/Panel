package finance

import (
	"fmt"
	"strings"
	"time"

	"panel/store"
)

// Summary 是给前端的财务概览。所有金额单位 = 分。
type Summary struct {
	AsOf         string `json:"as_of"`
	CurrentMonth string `json:"current_month"`

	Balance         int64 `json:"balance"`
	InitialBalance  int64 `json:"initial_balance"`
	TotalIncome     int64 `json:"total_income"`
	TotalBillSpent  int64 `json:"total_bill_spent"`
	TotalFixedSpent int64 `json:"total_fixed_spent"`
	TotalFreeSpent  int64 `json:"total_free_spent"`

	MonthExpected int64 `json:"month_expected"`
	MonthBill     int64 `json:"month_bill"`
	MonthFixed    int64 `json:"month_fixed"`
	MonthFree     int64 `json:"month_free"`
	MonthFreeCap  int64 `json:"month_free_cap"`
	MonthIncome   int64 `json:"month_income"`

	Shortfall         int64   `json:"shortfall"`
	EstExtraInterest  int64   `json:"est_extra_interest"`
	AssumedNMonths    int     `json:"assumed_n_months"`
	AssumedAnnualRate float64 `json:"assumed_annual_rate"`
	DebtInflated      bool    `json:"debt_inflated"`
}

// monthIndex 把 'YYYY-MM' 转为单调整数 (year*12 + month - 1),便于减法。
func monthIndex(yyyymm string) (int, error) {
	var y, m int
	if _, err := fmt.Sscanf(yyyymm, "%d-%d", &y, &m); err != nil {
		return 0, err
	}
	if m < 1 || m > 12 {
		return 0, fmt.Errorf("invalid month: %s", yyyymm)
	}
	return y*12 + m - 1, nil
}

// ComputeSummary 计算给定时刻 now 的财务概览。
func ComputeSummary(s *store.Store, now time.Time) (*Summary, error) {
	settings, err := s.GetFinanceSettings()
	if err != nil {
		return nil, err
	}
	bills, err := s.ListBills()
	if err != nil {
		return nil, err
	}
	fixeds, err := s.ListFixed()
	if err != nil {
		return nil, err
	}
	frees, err := s.ListFree()
	if err != nil {
		return nil, err
	}
	incomes, err := s.ListIncomes()
	if err != nil {
		return nil, err
	}

	asOf := now.Format("2006-01-02")
	currentMonth := now.Format("2006-01")
	currentIdx, _ := monthIndex(currentMonth)

	var totalIncome, monthIncome int64
	for _, inc := range incomes {
		totalIncome += inc.Amount
		if strings.HasPrefix(inc.Date, currentMonth) {
			monthIncome += inc.Amount
		}
	}

	var totalFreeSpent, monthFree int64
	for _, f := range frees {
		totalFreeSpent += f.Amount
		if strings.HasPrefix(f.Date, currentMonth) {
			monthFree += f.Amount
		}
	}

	var totalBillSpent, monthBill int64
	for _, b := range bills {
		startIdx, err := monthIndex(b.StartedMonth)
		if err != nil {
			continue
		}
		elapsed := currentIdx - startIdx + 1
		if elapsed < 0 {
			elapsed = 0
		}
		if elapsed > b.TotalMonths {
			elapsed = b.TotalMonths
		}
		totalBillSpent += b.MonthlyAmount * int64(elapsed)
		if currentIdx >= startIdx && currentIdx < startIdx+b.TotalMonths {
			monthBill += b.MonthlyAmount
		}
	}

	var totalFixedSpent, monthFixed int64
	for _, f := range fixeds {
		startIdx, err := monthIndex(f.StartedMonth)
		if err != nil {
			continue
		}
		elapsed := currentIdx - startIdx + 1
		if elapsed < 0 {
			elapsed = 0
		}
		totalFixedSpent += f.MonthlyAmount * int64(elapsed)
		if currentIdx >= startIdx {
			monthFixed += f.MonthlyAmount
		}
	}

	balance := settings.InitialBalance + totalIncome - totalBillSpent - totalFixedSpent - totalFreeSpent
	monthExpected := monthBill + monthFixed + monthFree

	shortfall := monthExpected - monthIncome
	if shortfall < 0 {
		shortfall = 0
	}
	var estExtra int64
	if shortfall > 0 && settings.AssumedNMonths > 0 {
		estExtra = int64(float64(shortfall) * settings.AssumedAnnualRate / float64(settings.AssumedNMonths))
	}

	return &Summary{
		AsOf:              asOf,
		CurrentMonth:      currentMonth,
		Balance:           balance,
		InitialBalance:    settings.InitialBalance,
		TotalIncome:       totalIncome,
		TotalBillSpent:    totalBillSpent,
		TotalFixedSpent:   totalFixedSpent,
		TotalFreeSpent:    totalFreeSpent,
		MonthExpected:     monthExpected,
		MonthBill:         monthBill,
		MonthFixed:        monthFixed,
		MonthFree:         monthFree,
		MonthFreeCap:      settings.FreeCapMonthly,
		MonthIncome:       monthIncome,
		Shortfall:         shortfall,
		EstExtraInterest:  estExtra,
		AssumedNMonths:    settings.AssumedNMonths,
		AssumedAnnualRate: settings.AssumedAnnualRate,
		DebtInflated:      shortfall > 0,
	}, nil
}
