package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

type WorkspaceCopyRequest struct {
	OperationID    string   `json:"operationId"`
	TargetMonth    string   `json:"targetMonth"`
	SourceRevision int64    `json:"sourceRevision"`
	TargetRevision int64    `json:"targetRevision"`
	TableIDs       []string `json:"tableIds"`
}

type WorkspaceCopyResult struct {
	OperationID    string         `json:"operationId"`
	Target         MonthWorkspace `json:"target"`
	CopiedTableIDs []string       `json:"copiedTableIds"`
}

func validateWorkspaceCopy(sourceMonth string, request WorkspaceCopyRequest) error {
	if _, err := ParseWorkspaceMonth(sourceMonth); err != nil {
		return err
	}
	if _, err := ParseWorkspaceMonth(request.TargetMonth); err != nil {
		return err
	}
	if sourceMonth == request.TargetMonth {
		return fmt.Errorf("%w：源月和目标月必须不同", ErrWorkspaceInvalid)
	}
	if strings.TrimSpace(request.OperationID) == "" || len(request.OperationID) > 128 {
		return fmt.Errorf("%w：operationId 不能为空或超过 128 字节", ErrWorkspaceInvalid)
	}
	if request.SourceRevision < 0 || request.TargetRevision < 0 || request.TargetRevision == math.MaxInt64 {
		return fmt.Errorf("%w：revision 无效", ErrWorkspaceInvalid)
	}
	if len(request.TableIDs) == 0 || len(request.TableIDs) > MaxWorkspaceTables {
		return fmt.Errorf("%w：请选择 1 到 %d 张表", ErrWorkspaceInvalid, MaxWorkspaceTables)
	}
	seen := make(map[string]bool, len(request.TableIDs))
	for _, id := range request.TableIDs {
		if strings.TrimSpace(id) == "" || len(id) > 128 || seen[id] {
			return fmt.Errorf("%w：所选表 ID 不能为空、重复或超过 128 字节", ErrWorkspaceInvalid)
		}
		seen[id] = true
	}
	return nil
}

func workspaceCopyFingerprint(sourceMonth string, request WorkspaceCopyRequest) (string, error) {
	payload := struct {
		SourceMonth string `json:"sourceMonth"`
		WorkspaceCopyRequest
	}{SourceMonth: sourceMonth, WorkspaceCopyRequest: request}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

// CopyWorkspaceTables appends selected tables atomically and leaves the source
// unchanged. The durable operation record makes a retry safe after a lost
// response or process restart, even if either month has since been edited.
func (s *Store) CopyWorkspaceTables(ctx context.Context, sourceMonth string, request WorkspaceCopyRequest) (WorkspaceCopyResult, error) {
	if err := validateWorkspaceCopy(sourceMonth, request); err != nil {
		return WorkspaceCopyResult{}, err
	}
	fingerprint, err := workspaceCopyFingerprint(sourceMonth, request)
	if err != nil {
		return WorkspaceCopyResult{}, err
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return WorkspaceCopyResult{}, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return WorkspaceCopyResult{}, err
	}
	// Acquire the write reservation before reading either revision or the
	// idempotency record. A concurrent copy/save cannot pass stale checks.
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return WorkspaceCopyResult{}, fmt.Errorf("begin workspace copy: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			// The request context may have been canceled after a disconnected
			// client. Roll back on a fresh context before returning the connection.
			rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = conn.ExecContext(rollbackCtx, `ROLLBACK`)
		}
	}()

	var recordedFingerprint, recordedMonth, copiedJSON string
	err = conn.QueryRowContext(ctx,
		`SELECT fingerprint, target_month, copied_table_ids_json
		 FROM short_term_workspace_copies WHERE operation_id = ?`, request.OperationID).
		Scan(&recordedFingerprint, &recordedMonth, &copiedJSON)
	var copiedIDs []string
	var target MonthWorkspace
	switch {
	case err == nil:
		if recordedFingerprint != fingerprint || recordedMonth != request.TargetMonth {
			return WorkspaceCopyResult{}, fmt.Errorf("%w：operationId 已用于另一份迁移请求", ErrWorkspaceInvalid)
		}
		if err := json.Unmarshal([]byte(copiedJSON), &copiedIDs); err != nil {
			return WorkspaceCopyResult{}, fmt.Errorf("decode workspace copy record: %w", err)
		}
		// Replays return the current target, not an obsolete saved snapshot.
		// Source membership/revision is intentionally not checked again.
		target, err = getMonthWorkspace(ctx, conn, recordedMonth)
		if err != nil {
			return WorkspaceCopyResult{}, err
		}
	case errors.Is(err, sql.ErrNoRows):
		source, err := getMonthWorkspace(ctx, conn, sourceMonth)
		if err != nil {
			return WorkspaceCopyResult{}, err
		}
		target, err = getMonthWorkspace(ctx, conn, request.TargetMonth)
		if err != nil {
			return WorkspaceCopyResult{}, err
		}
		if source.Revision != request.SourceRevision || target.Revision != request.TargetRevision {
			return WorkspaceCopyResult{}, fmt.Errorf("%w：源月或目标月已有其他修改", ErrWorkspaceConflict)
		}
		copiedIDs, err = appendWorkspaceCopies(source, &target, request.TableIDs)
		if err != nil {
			return WorkspaceCopyResult{}, err
		}
		raw, err := marshalWorkspaceTables(target.Tables)
		if err != nil {
			return WorkspaceCopyResult{}, err
		}
		target.Revision, err = saveWorkspaceSnapshot(ctx, conn, target.Month, target.Revision, raw)
		if err != nil {
			return WorkspaceCopyResult{}, err
		}
		idsJSON, err := json.Marshal(copiedIDs)
		if err != nil {
			return WorkspaceCopyResult{}, err
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO short_term_workspace_copies (operation_id, fingerprint, target_month, copied_table_ids_json)
			 VALUES (?, ?, ?, ?)`, request.OperationID, fingerprint, target.Month, string(idsJSON)); err != nil {
			return WorkspaceCopyResult{}, fmt.Errorf("record workspace copy: %w", err)
		}
	default:
		return WorkspaceCopyResult{}, fmt.Errorf("read workspace copy record: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return WorkspaceCopyResult{}, fmt.Errorf("commit workspace copy: %w", err)
	}
	committed = true
	return WorkspaceCopyResult{OperationID: request.OperationID, Target: target, CopiedTableIDs: copiedIDs}, nil
}

func appendWorkspaceCopies(source MonthWorkspace, target *MonthWorkspace, selectedIDs []string) ([]string, error) {
	selected := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		selected[id] = true
	}
	usedTableIDs, usedRowIDs := make(map[string]bool), make(map[string]bool)
	for _, workspace := range []MonthWorkspace{source, *target} {
		for _, table := range workspace.Tables {
			usedTableIDs[table.ID] = true
			for _, row := range table.Rows {
				usedRowIDs[row.ID] = true
			}
		}
	}
	copiedIDs := make([]string, 0, len(selectedIDs))
	// Iterate the source's order, independent of checkbox selection order.
	for _, table := range source.Tables {
		if !selected[table.ID] {
			continue
		}
		copy := WorkspaceTable{ID: newWorkspaceCopyID(usedTableIDs), Header: table.Header, Rows: make([]WorkspaceRow, len(table.Rows))}
		for i, row := range table.Rows {
			copy.Rows[i] = WorkspaceRow{ID: newWorkspaceCopyID(usedRowIDs), Content: row.Content}
		}
		target.Tables = append(target.Tables, copy)
		copiedIDs = append(copiedIDs, copy.ID)
	}
	if len(copiedIDs) != len(selectedIDs) {
		return nil, fmt.Errorf("%w：所选表不属于源月，请重新选择", ErrWorkspaceInvalid)
	}
	return copiedIDs, nil
}

func newWorkspaceCopyID(used map[string]bool) string {
	for {
		id := uuid.NewString()
		if !used[id] {
			used[id] = true
			return id
		}
	}
}
