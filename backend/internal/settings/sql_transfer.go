package settings

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
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

type SQLTransfer struct {
	db *sql.DB
}

func NewSQLTransfer(db *sql.DB) *SQLTransfer {
	return &SQLTransfer{db: db}
}

func (t *SQLTransfer) Export() ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("BEGIN TRANSACTION;\n")

	for _, table := range ownedTableNames {
		createSQL, err := lookupCreateTableSQL(t.db, table)
		if err != nil {
			return nil, err
		}
		builder.WriteString(createSQL)
		builder.WriteString(";\n")

		columns, err := lookupTableColumns(t.db, table)
		if err != nil {
			return nil, err
		}
		queryParts := make([]string, 0, len(columns))
		columnNames := make([]string, 0, len(columns))
		for _, column := range columns {
			queryParts = append(queryParts, fmt.Sprintf("quote(%s)", quoteIdentifier(column)))
			columnNames = append(columnNames, quoteIdentifier(column))
		}

		rows, err := t.db.Query(
			fmt.Sprintf("SELECT %s FROM %s ORDER BY rowid ASC", strings.Join(queryParts, ", "), quoteIdentifier(table)),
		)
		if err != nil {
			return nil, fmt.Errorf("query %s rows for export: %w", table, err)
		}

		for rows.Next() {
			values := make([]sql.NullString, len(columns))
			targets := make([]any, len(columns))
			for index := range values {
				targets[index] = &values[index]
			}
			if err := rows.Scan(targets...); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan %s export row: %w", table, err)
			}

			literals := make([]string, 0, len(values))
			for _, value := range values {
				if !value.Valid {
					literals = append(literals, "NULL")
					continue
				}
				literals = append(literals, value.String)
			}

			builder.WriteString(fmt.Sprintf(
				"INSERT INTO %s (%s) VALUES (%s);\n",
				quoteIdentifier(table),
				strings.Join(columnNames, ", "),
				strings.Join(literals, ", "),
			))
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate %s export rows: %w", table, err)
		}
		rows.Close()
	}

	builder.WriteString("COMMIT;\n")
	return []byte(builder.String()), nil
}

func (t *SQLTransfer) Import(raw []byte) error {
	tempFile, err := os.CreateTemp("", "aigate-import-*.sqlite")
	if err != nil {
		return fmt.Errorf("create temp import db path: %w", err)
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp import db path: %w", err)
	}
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)

	tempDB, err := sql.Open("sqlite", tempPath)
	if err != nil {
		return fmt.Errorf("open temp import db: %w", err)
	}
	defer tempDB.Close()

	if _, err := tempDB.Exec(string(raw)); err != nil {
		return fmt.Errorf("exec import SQL: %w", err)
	}

	for _, table := range ownedTableNames {
		if _, err := lookupCreateTableSQL(tempDB, table); err != nil {
			return err
		}
	}

	return replaceOwnedTablesFromDatabase(t.db, tempPath)
}

func lookupCreateTableSQL(db *sql.DB, table string) (string, error) {
	var createSQL string
	if err := db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`,
		table,
	).Scan(&createSQL); err != nil {
		return "", fmt.Errorf("lookup create SQL for %s: %w", table, err)
	}
	return createSQL, nil
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
