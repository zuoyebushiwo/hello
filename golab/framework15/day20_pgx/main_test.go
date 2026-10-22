package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestLoadOrdersBatches(t *testing.T) {
	db, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	rows := db.NewRows([]string{"customer_id", "id", "amount"}).
		AddRow("c-1", "o-1", int64(100)).
		AddRow("c-2", "o-2", int64(200))
	db.ExpectQuery(ordersByCustomersSQL).WithArgs(pgxmock.AnyArg()).WillReturnRows(rows)

	orders, err := LoadOrders(context.Background(), db, []string{"c-1", "c-2"})
	require.NoError(t, err)
	require.Len(t, orders["c-1"], 1)
	require.Len(t, orders["c-2"], 1)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCreateOrderCommits(t *testing.T) {
	db, _ := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	defer db.Close()
	db.ExpectBegin()
	db.ExpectExec(insertOrderSQL).WithArgs("o-1", "c-1", int64(10)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec(insertAuditSQL).WithArgs("o-1", "created").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()

	require.NoError(t, CreateOrder(context.Background(), db, Order{ID: "o-1", CustomerID: "c-1", Amount: 10}))
	require.NoError(t, db.ExpectationsWereMet())
}

func TestCreateOrderRollsBack(t *testing.T) {
	db, _ := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	defer db.Close()
	db.ExpectBegin()
	db.ExpectExec(insertOrderSQL).WithArgs("o-1", "c-1", int64(10)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec(insertAuditSQL).WithArgs("o-1", "created").WillReturnError(errors.New("audit down"))
	db.ExpectRollback()

	err := CreateOrder(context.Background(), db, Order{ID: "o-1", CustomerID: "c-1", Amount: 10})
	require.Error(t, err)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestBulkAudit(t *testing.T) {
	db, _ := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	defer db.Close()
	db.ExpectCopyFrom(pgx.Identifier{"audit_log"}, []string{"order_id", "action"}).WillReturnResult(2)

	n, err := BulkAudit(context.Background(), db, []AuditEntry{{"o-1", "created"}, {"o-2", "created"}})
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
	require.NoError(t, db.ExpectationsWereMet())
}
