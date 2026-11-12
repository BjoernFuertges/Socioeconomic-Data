package handlers

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
)

// unlimitedRowsCap is the effective limit=null cap. It starts at the floor
// maxUnlimitedRows and is raised once at startup by ConfigureUnlimitedRowsCap.
var unlimitedRowsCap atomic.Int64

func init() { unlimitedRowsCap.Store(maxUnlimitedRows) }

// UnlimitedRowsCap returns the current maximum number of rows a limit=null
// request may return.
func UnlimitedRowsCap() int64 { return unlimitedRowsCap.Load() }

// capForLargestTable derives the cap from the row count of the largest table:
// 1.25x headroom, never below the floor (also covers reltuples = -1 for tables
// that were never analyzed, and an empty database).
func capForLargestTable(largestRows int64) int64 {
	if c := largestRows + largestRows/4; c > maxUnlimitedRows {
		return c
	}
	return maxUnlimitedRows
}

// ConfigureUnlimitedRowsCap sizes the limit=null cap from the largest dataset
// table (planner estimate pg_class.reltuples, no count(*)): the stg_* tables in
// transformed_data and the snap_results_* snapshots. Other tables (geometry,
// crosswalk, marts) are never served via limit=null and must not raise the cap.
// Staging models that are views have no estimate; their backing snapshots
// count (and hold history, so they are an upper bound). Call once at startup;
// on error the floor stays in effect. Returns the effective cap.
func ConfigureUnlimitedRowsCap(db *sqlx.DB) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var largest int64
	err := db.GetContext(ctx, &largest, `
		SELECT COALESCE(MAX(c.reltuples), 0)::bigint
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'm')
		  AND ((n.nspname = 'transformed_data' AND c.relname LIKE 'stg\_%')
		    OR (n.nspname = 'snapshots' AND c.relname LIKE 'snap\_results\_%'))`)
	if err != nil {
		return UnlimitedRowsCap(), err
	}
	c := capForLargestTable(largest)
	unlimitedRowsCap.Store(c)
	return c, nil
}
