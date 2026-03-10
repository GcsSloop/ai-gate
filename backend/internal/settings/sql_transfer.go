package settings

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

var ownedTableNames = []string{
	"accounts",
	"account_usage_snapshots",
	"conversations",
	"messages",
	"runs",
	"app_settings",
	"failover_queue_items",
}

var accountExchangeTableNames = []string{
	"accounts",
	"account_usage_snapshots",
	"failover_queue_items",
}

const accountUsageSnapshotExportLimit = 2000

type SQLTransfer struct {
	db *sql.DB
}

type databaseExchangePayload struct {
	Format  string                           `json:"format"`
	Version int                              `json:"version"`
	Tables  map[string]databaseExchangeTable `json:"tables"`
}

type databaseExchangeTable struct {
	Columns []string                  `json:"columns"`
	Rows    [][]databaseExchangeValue `json:"rows"`
}

type databaseExchangeValue struct {
	Type  string `json:"type"`
	Value any    `json:"value,omitempty"`
}

func NewSQLTransfer(db *sql.DB) *SQLTransfer {
	return &SQLTransfer{db: db}
}

func (t *SQLTransfer) Export() ([]byte, error) {
	payload := databaseExchangePayload{
		Format:  "aigate-db-exchange",
		Version: 1,
		Tables:  make(map[string]databaseExchangeTable, len(accountExchangeTableNames)),
	}
	for _, table := range accountExchangeTableNames {
		columns, err := lookupTableColumns(t.db, table)
		if err != nil {
			return nil, err
		}
		rowsPayload := make([][]databaseExchangeValue, 0)
		query := fmt.Sprintf("SELECT %s FROM %s ORDER BY rowid ASC", strings.Join(quoteIdentifiers(columns), ", "), quoteIdentifier(table))
		if table == "account_usage_snapshots" {
			query = fmt.Sprintf(
				`WITH ranked AS (
					SELECT %s,
						CASE
							WHEN datetime(COALESCE(checked_at, '1970-01-01T00:00:00Z')) >= datetime('now', '-7 day') THEN 'recent:' || CAST(id AS TEXT)
							WHEN datetime(COALESCE(checked_at, '1970-01-01T00:00:00Z')) >= datetime('now', '-30 day') THEN 'mid:' || strftime('%%Y-%%m-%%d', COALESCE(checked_at, '1970-01-01T00:00:00Z')) || ':' || printf('%%02d', (CAST(strftime('%%H', COALESCE(checked_at, '1970-01-01T00:00:00Z')) AS INTEGER) / 6) * 6)
							ELSE 'old:' || strftime('%%Y-%%m-%%d', COALESCE(checked_at, '1970-01-01T00:00:00Z'))
						END AS bucket_key,
						ROW_NUMBER() OVER (
							PARTITION BY
								CASE
									WHEN datetime(COALESCE(checked_at, '1970-01-01T00:00:00Z')) >= datetime('now', '-7 day') THEN 'recent:' || CAST(id AS TEXT)
									WHEN datetime(COALESCE(checked_at, '1970-01-01T00:00:00Z')) >= datetime('now', '-30 day') THEN 'mid:' || strftime('%%Y-%%m-%%d', COALESCE(checked_at, '1970-01-01T00:00:00Z')) || ':' || printf('%%02d', (CAST(strftime('%%H', COALESCE(checked_at, '1970-01-01T00:00:00Z')) AS INTEGER) / 6) * 6)
									ELSE 'old:' || strftime('%%Y-%%m-%%d', COALESCE(checked_at, '1970-01-01T00:00:00Z'))
								END
							ORDER BY datetime(COALESCE(checked_at, '1970-01-01T00:00:00Z')) DESC, id DESC
						) AS bucket_rank
					FROM %s
				),
				sampled AS (
					SELECT %s
					FROM ranked
					WHERE bucket_rank = 1
					ORDER BY datetime(COALESCE(checked_at, '1970-01-01T00:00:00Z')) DESC, id DESC
					LIMIT %d
				)
				SELECT %s
				FROM sampled
				ORDER BY datetime(COALESCE(checked_at, '1970-01-01T00:00:00Z')) ASC, id ASC`,
				strings.Join(quoteIdentifiers(columns), ", "),
				quoteIdentifier(table),
				strings.Join(quoteIdentifiers(columns), ", "),
				accountUsageSnapshotExportLimit,
				strings.Join(quoteIdentifiers(columns), ", "),
			)
		}
		rows, err := t.db.Query(query)
		if err != nil {
			return nil, fmt.Errorf("query %s rows for export: %w", table, err)
		}

		for rows.Next() {
			scanValues := make([]any, len(columns))
			targets := make([]any, len(columns))
			for index := range scanValues {
				targets[index] = &scanValues[index]
			}
			if err := rows.Scan(targets...); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan %s export row: %w", table, err)
			}
			rowPayload := make([]databaseExchangeValue, 0, len(scanValues))
			for _, value := range scanValues {
				rowPayload = append(rowPayload, encodeExchangeValue(value))
			}
			rowsPayload = append(rowsPayload, rowPayload)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate %s export rows: %w", table, err)
		}
		rows.Close()
		payload.Tables[table] = databaseExchangeTable{
			Columns: columns,
			Rows:    rowsPayload,
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal exchange payload: %w", err)
	}
	return raw, nil
}

func (t *SQLTransfer) Import(raw []byte) error {
	var payload databaseExchangePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode import JSON: %w", err)
	}
	if payload.Format != "aigate-db-exchange" {
		return fmt.Errorf("unsupported import format: %s", payload.Format)
	}
	if payload.Version != 1 {
		return fmt.Errorf("unsupported import version: %d", payload.Version)
	}
	return replaceOwnedTablesFromPayload(t.db, payload.Tables, accountExchangeTableNames)
}

func lookupTableColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(table)))
	if err != nil {
		return nil, fmt.Errorf("inspect table %s columns: %w", table, err)
	}
	return scanTableColumns(rows, table)
}

func lookupTableColumnsInSchema(tx *sql.Tx, schema string, table string) ([]string, error) {
	pragma := fmt.Sprintf("PRAGMA %s.table_info(%s)", quoteIdentifier(schema), quoteIdentifier(table))
	if schema == "" {
		pragma = fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(table))
	}
	rows, err := tx.Query(pragma)
	if err != nil {
		return nil, fmt.Errorf("inspect table %s columns: %w", table, err)
	}
	return scanTableColumns(rows, table)
}

func scanTableColumns(rows *sql.Rows, table string) ([]string, error) {
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan table info for %s: %w", table, err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table info for %s: %w", table, err)
	}
	return columns, nil
}

func replaceOwnedTablesFromPayload(target *sql.DB, sourceTables map[string]databaseExchangeTable, tableNames []string) (err error) {
	tx, err := target.Begin()
	if err != nil {
		return fmt.Errorf("begin replace owned tables: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for index := len(tableNames) - 1; index >= 0; index-- {
		table := tableNames[index]
		if _, err = tx.Exec(fmt.Sprintf("DELETE FROM %s", quoteIdentifier(table))); err != nil {
			return fmt.Errorf("clear %s before import: %w", table, err)
		}
	}
	for _, table := range tableNames {
		sourceTable, ok := sourceTables[table]
		if !ok {
			continue
		}
		targetColumns, colErr := lookupTableColumnsInSchema(tx, "", table)
		if colErr != nil {
			return colErr
		}
		sourceColumns := sourceTable.Columns
		commonColumns := commonTableColumns(targetColumns, sourceColumns)
		if len(commonColumns) == 0 {
			return fmt.Errorf("copy imported rows into %s: no common columns found", table)
		}
		columnIndex := make(map[string]int, len(sourceColumns))
		for idx, column := range sourceColumns {
			columnIndex[column] = idx
		}

		insertColumns := make([]string, 0, len(commonColumns))
		for _, column := range commonColumns {
			insertColumns = append(insertColumns, quoteIdentifier(column))
		}
		placeholders := make([]string, 0, len(commonColumns))
		for range commonColumns {
			placeholders = append(placeholders, "?")
		}
		insertSQL := fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)",
			quoteIdentifier(table),
			strings.Join(insertColumns, ", "),
			strings.Join(placeholders, ", "),
		)

		for rowIndex, row := range sourceTable.Rows {
			values := make([]any, 0, len(commonColumns))
			for _, column := range commonColumns {
				sourceIndex := columnIndex[column]
				if sourceIndex >= len(row) {
					values = append(values, nil)
					continue
				}
				decoded, decodeErr := decodeExchangeValue(row[sourceIndex])
				if decodeErr != nil {
					return fmt.Errorf("decode %s row %d column %s: %w", table, rowIndex, column, decodeErr)
				}
				values = append(values, decoded)
			}
			if _, err = tx.Exec(insertSQL, values...); err != nil {
				return fmt.Errorf("copy imported rows into %s: %w", table, err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit imported data: %w", err)
	}
	return nil
}

func quoteIdentifiers(values []string) []string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, quoteIdentifier(value))
	}
	return quoted
}

func encodeExchangeValue(value any) databaseExchangeValue {
	switch typed := value.(type) {
	case nil:
		return databaseExchangeValue{Type: "null"}
	case int64:
		return databaseExchangeValue{Type: "integer", Value: strconv.FormatInt(typed, 10)}
	case float64:
		return databaseExchangeValue{Type: "real", Value: typed}
	case bool:
		return databaseExchangeValue{Type: "bool", Value: typed}
	case []byte:
		return databaseExchangeValue{Type: "bytes", Value: base64.StdEncoding.EncodeToString(typed)}
	case string:
		return databaseExchangeValue{Type: "text", Value: typed}
	default:
		return databaseExchangeValue{Type: "text", Value: fmt.Sprint(typed)}
	}
}

func decodeExchangeValue(value databaseExchangeValue) (any, error) {
	switch value.Type {
	case "null":
		return nil, nil
	case "integer":
		switch raw := value.Value.(type) {
		case string:
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, err
			}
			return parsed, nil
		case float64:
			return int64(raw), nil
		default:
			return nil, fmt.Errorf("invalid integer value type %T", value.Value)
		}
	case "real":
		switch raw := value.Value.(type) {
		case float64:
			return raw, nil
		case string:
			parsed, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return nil, err
			}
			return parsed, nil
		default:
			return nil, fmt.Errorf("invalid real value type %T", value.Value)
		}
	case "bool":
		raw, ok := value.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("invalid bool value type %T", value.Value)
		}
		return raw, nil
	case "bytes":
		raw, ok := value.Value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid bytes value type %T", value.Value)
		}
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, err
		}
		return decoded, nil
	case "text":
		if value.Value == nil {
			return "", nil
		}
		raw, ok := value.Value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid text value type %T", value.Value)
		}
		return raw, nil
	default:
		return nil, fmt.Errorf("unknown value type %q", value.Type)
	}
}

func replaceOwnedTablesFromDatabase(target *sql.DB, sourcePath string) (err error) {
	tx, err := target.Begin()
	if err != nil {
		return fmt.Errorf("begin replace owned tables: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS imported", escapeSQLiteString(sourcePath))); err != nil {
		return fmt.Errorf("attach import database: %w", err)
	}
	defer func() {
		_, _ = tx.Exec("DETACH DATABASE imported")
	}()

	for index := len(ownedTableNames) - 1; index >= 0; index-- {
		table := ownedTableNames[index]
		if _, err = tx.Exec(fmt.Sprintf("DELETE FROM %s", quoteIdentifier(table))); err != nil {
			return fmt.Errorf("clear %s before import: %w", table, err)
		}
	}
	for _, table := range ownedTableNames {
		targetColumns, colErr := lookupTableColumnsInSchema(tx, "", table)
		if colErr != nil {
			return colErr
		}
		sourceColumns, colErr := lookupTableColumnsInSchema(tx, "imported", table)
		if colErr != nil {
			return colErr
		}
		commonColumns := commonTableColumns(targetColumns, sourceColumns)
		if len(commonColumns) == 0 {
			return fmt.Errorf("copy imported rows into %s: no common columns found", table)
		}

		insertColumns := make([]string, 0, len(commonColumns))
		selectColumns := make([]string, 0, len(commonColumns))
		for _, column := range commonColumns {
			quoted := quoteIdentifier(column)
			insertColumns = append(insertColumns, quoted)
			selectColumns = append(selectColumns, quoted)
		}

		if _, err = tx.Exec(
			fmt.Sprintf(
				"INSERT INTO %s (%s) SELECT %s FROM imported.%s",
				quoteIdentifier(table),
				strings.Join(insertColumns, ", "),
				strings.Join(selectColumns, ", "),
				quoteIdentifier(table),
			),
		); err != nil {
			return fmt.Errorf("copy imported rows into %s: %w", table, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit imported data: %w", err)
	}
	return nil
}

func commonTableColumns(targetColumns []string, sourceColumns []string) []string {
	sourceSet := make(map[string]struct{}, len(sourceColumns))
	for _, column := range sourceColumns {
		sourceSet[column] = struct{}{}
	}
	result := make([]string, 0, len(targetColumns))
	for _, column := range targetColumns {
		if _, ok := sourceSet[column]; ok {
			result = append(result, column)
		}
	}
	return result
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func escapeSQLiteString(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}
