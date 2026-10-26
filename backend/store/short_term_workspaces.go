package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	MaxWorkspaceBytes  = 4 << 20
	MaxWorkspaceTables = 100
	MaxWorkspaceRows   = 1000
)

var (
	ErrWorkspaceInvalid  = errors.New("短期工作区内容无效")
	ErrWorkspaceConflict = errors.New("此月工作区已更新，请保留当前内容并重新载入")
)

type WorkspaceRow struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type WorkspaceTable struct {
	ID     string         `json:"id"`
	Header string         `json:"header"`
	Rows   []WorkspaceRow `json:"rows"`
}

type MonthWorkspace struct {
	Month    string           `json:"month"`
	Revision int64            `json:"revision"`
	Tables   []WorkspaceTable `json:"tables"`
}

func ParseWorkspaceMonth(month string) (time.Time, error) {
	parsed, err := time.Parse("2006-01", month)
	if err != nil || len(month) != 7 || parsed.Year() < 1 || parsed.Format("2006-01") != month {
		return time.Time{}, fmt.Errorf("%w：月份必须为有效的 YYYY-MM", ErrWorkspaceInvalid)
	}
	return parsed, nil
}

// GetMonthWorkspace does not create a record for an untouched month.
func (s *Store) GetMonthWorkspace(ctx context.Context, month string) (MonthWorkspace, error) {
	if _, err := ParseWorkspaceMonth(month); err != nil {
		return MonthWorkspace{}, err
	}
	workspace := MonthWorkspace{Month: month, Tables: []WorkspaceTable{}}
	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT revision, tables_json FROM short_term_workspace_months WHERE month = ?`, month).Scan(&workspace.Revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return workspace, nil
	}
	if err != nil {
		return MonthWorkspace{}, fmt.Errorf("read short-term workspace: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &workspace.Tables); err != nil {
		return MonthWorkspace{}, fmt.Errorf("decode short-term workspace %s: %w", month, err)
	}
	return workspace, nil
}

// SaveMonthWorkspace commits headers, ordered tables, and rows together using
// one revision per month. Empty months remain versioned to reject stale saves.
func (s *Store) SaveMonthWorkspace(ctx context.Context, month string, revision int64, tables []WorkspaceTable) (MonthWorkspace, error) {
	if _, err := ParseWorkspaceMonth(month); err != nil {
		return MonthWorkspace{}, err
	}
	if revision < 0 || revision == math.MaxInt64 {
		return MonthWorkspace{}, fmt.Errorf("%w：revision 无效", ErrWorkspaceInvalid)
	}
	if tables == nil || len(tables) > MaxWorkspaceTables {
		return MonthWorkspace{}, fmt.Errorf("%w：tables 必须为数组且不超过 %d 张表", ErrWorkspaceInvalid, MaxWorkspaceTables)
	}
	tableIDs := make(map[string]bool, len(tables))
	rowCount := 0
	for _, table := range tables {
		if strings.TrimSpace(table.ID) == "" || len(table.ID) > 128 || tableIDs[table.ID] {
			return MonthWorkspace{}, fmt.Errorf("%w：表 ID 不能为空、重复或超过 128 字节", ErrWorkspaceInvalid)
		}
		tableIDs[table.ID] = true
		if table.Rows == nil {
			return MonthWorkspace{}, fmt.Errorf("%w：每张表的 rows 必须为数组", ErrWorkspaceInvalid)
		}
		rowCount += len(table.Rows)
		if rowCount > MaxWorkspaceRows {
			return MonthWorkspace{}, fmt.Errorf("%w：每月总行数不能超过 %d", ErrWorkspaceInvalid, MaxWorkspaceRows)
		}
		rowIDs := make(map[string]bool, len(table.Rows))
		for _, row := range table.Rows {
			if strings.TrimSpace(row.ID) == "" || len(row.ID) > 128 || rowIDs[row.ID] {
				return MonthWorkspace{}, fmt.Errorf("%w：同表内的行 ID 不能为空、重复或超过 128 字节", ErrWorkspaceInvalid)
			}
			rowIDs[row.ID] = true
		}
	}
	raw, err := json.Marshal(tables)
	if err != nil {
		return MonthWorkspace{}, err
	}
	if len(raw) > MaxWorkspaceBytes {
		return MonthWorkspace{}, fmt.Errorf("%w：每月内容超过 4 MiB", ErrWorkspaceInvalid)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return MonthWorkspace{}, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return MonthWorkspace{}, err
	}
	var nextRevision int64
	if revision == 0 {
		err = conn.QueryRowContext(ctx,
			`INSERT INTO short_term_workspace_months (month, revision, tables_json) VALUES (?, 1, ?)
			 ON CONFLICT(month) DO NOTHING RETURNING revision`, month, string(raw)).Scan(&nextRevision)
	} else {
		err = conn.QueryRowContext(ctx,
			`UPDATE short_term_workspace_months SET revision = revision + 1, tables_json = ?
			 WHERE month = ? AND revision = ? RETURNING revision`, string(raw), month, revision).Scan(&nextRevision)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return MonthWorkspace{}, ErrWorkspaceConflict
	}
	if err != nil {
		return MonthWorkspace{}, fmt.Errorf("save short-term workspace: %w", err)
	}
	return MonthWorkspace{Month: month, Revision: nextRevision, Tables: tables}, nil
}
