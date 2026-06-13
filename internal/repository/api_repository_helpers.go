package repository

import (
	"context"
	"encoding/json"
	"fmt"
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
		raw := strings.TrimSpace(v)
		var parsed any
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			return raw
		}
		b, _ := json.Marshal(raw)
		return string(b)
	default:
		return value
	}
}

func normalizeSettingKey(value any) string {
	return stripOuterQuotes(valueString(value))
}

func valueString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return strings.TrimSpace(strings.ReplaceAll(strings.Trim(fmt.Sprint(v), `"`), `\"`, `"`))
	}
}

func stripOuterQuotes(value string) string {
	raw := strings.TrimSpace(value)
	for {
		if len(raw) < 2 {
			return raw
		}
		first := raw[0]
		last := raw[len(raw)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') || (first == '`' && last == '`') {
			raw = strings.TrimSpace(raw[1 : len(raw)-1])
			continue
		}
		return raw
	}
}
