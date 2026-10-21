// Package tx is a tiny in-memory database used to make transaction boundaries,
// rollback, snapshot reads and batch-vs-N+1 visible in one process.
package tx

import (
	"errors"
	"fmt"
	"sync"
)

type Row map[string]string

type Stats struct {
	Statements   int
	Transactions int
	Commits      int
	Rollbacks    int
}

type DB struct {
	mu     sync.Mutex
	tables map[string][]Row
	stats  Stats
}

func NewDB() *DB { return &DB{tables: make(map[string][]Row)} }

type op struct {
	table string
	row   Row
}

type Tx struct {
	db       *DB
	snapshot map[string][]Row
	local    map[string][]Row
	ops      []op
	active   bool
}

var ErrTxDone = errors.New("transaction is already finished")
var ErrNameRequired = errors.New("row.name is required")

func validate(row Row) error {
	if row["name"] == "" {
		return ErrNameRequired
	}
	return nil
}

func copyRow(row Row) Row {
	out := make(Row, len(row))
	for k, v := range row {
		out[k] = v
	}
	return out
}

func copyRows(rows []Row) []Row {
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, copyRow(r))
	}
	return out
}

func copyTables(tables map[string][]Row) map[string][]Row {
	out := make(map[string][]Row, len(tables))
	for name, rows := range tables {
		out[name] = copyRows(rows)
	}
	return out
}

func (db *DB) Begin() *Tx {
	db.mu.Lock()
	defer db.mu.Unlock()
	return &Tx{
		db:       db,
		snapshot: copyTables(db.tables),
		local:    copyTables(db.tables),
		active:   true,
	}
}

func (tx *Tx) Insert(table string, row Row) error {
	if !tx.active {
		return ErrTxDone
	}
	if err := validate(row); err != nil {
		return fmt.Errorf("insert into %s: %w", table, err)
	}
	cp := copyRow(row)
	tx.local[table] = append(tx.local[table], cp)
	tx.ops = append(tx.ops, op{table: table, row: cp})
	return nil
}

// Select reads the snapshot plus this transaction's own writes. It never sees
// another transaction's uncommitted or later-committed rows.
func (tx *Tx) Select(table string) []Row { return copyRows(tx.local[table]) }

func (tx *Tx) Commit() error {
	if !tx.active {
		return ErrTxDone
	}
	tx.active = false
	tx.db.mu.Lock()
	defer tx.db.mu.Unlock()
	for _, o := range tx.ops {
		tx.db.tables[o.table] = append(tx.db.tables[o.table], copyRow(o.row))
	}
	tx.db.stats.Statements += len(tx.ops)
	tx.db.stats.Transactions++
	tx.db.stats.Commits++
	return nil
}

func (tx *Tx) Rollback() {
	if !tx.active {
		return
	}
	tx.active = false
	tx.db.mu.Lock()
	tx.db.stats.Rollbacks++
	tx.db.mu.Unlock()
}

func (db *DB) Rows(table string) []Row {
	db.mu.Lock()
	defer db.mu.Unlock()
	return copyRows(db.tables[table])
}

func (db *DB) Stats() Stats {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.stats
}

// InsertOneByOne is the N+1 shape: one transaction and one statement per row.
func (db *DB) InsertOneByOne(table string, rows []Row) error {
	for _, row := range rows {
		tx := db.Begin()
		if err := tx.Insert(table, row); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// BatchInsert is the shape you want for 1000 rows: one statement, one commit.
func (db *DB) BatchInsert(table string, rows []Row) error {
	for _, row := range rows {
		if err := validate(row); err != nil {
			return fmt.Errorf("batch insert into %s: %w", table, err)
		}
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	db.tables[table] = append(db.tables[table], copyRows(rows)...)
	db.stats.Statements++
	db.stats.Transactions++
	db.stats.Commits++
	return nil
}
