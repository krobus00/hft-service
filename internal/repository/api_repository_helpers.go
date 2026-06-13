package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

func selectMaps(ctx context.Context, db *sqlx.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		item := map[string]any{}
		if err := rows.MapScan(item); err != nil {
			return nil, err
		}
		for key, value := range item {
			item[key] = normalizeDBValue(value)
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func normalizeDBValue(value any) any {
	switch v := value.(type) {
	case []byte:
		return string(v)
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	default:
		return value
	}
}

func normalizeConfigValue(value any) any {
	switch v := value.(type) {
	case map[string]any, []any:
		b, _ := json.Marshal(v)
		return string(b)
	case string:
		return strings.TrimSpace(v)
	default:
		return value
	}
}
