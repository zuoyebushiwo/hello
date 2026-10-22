package main

import (
	"context"
	"fmt"
	"sort"

	"frameworklab/internal/labkit"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
)

type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

type Order struct {
	CustomerID string
	ID         string
	Amount     int64
}

type AuditEntry struct {
	OrderID string
	Action  string
}

const ordersByCustomersSQL = `SELECT customer_id, id, amount FROM orders WHERE customer_id = ANY($1) ORDER BY customer_id, id`
const insertOrderSQL = `INSERT INTO orders (id, customer_id, amount) VALUES ($1, $2, $3)`
const insertAuditSQL = `INSERT INTO audit_log (order_id, action) VALUES ($1, $2)`

func LoadOrders(ctx context.Context, db DB, customerIDs []string) (map[string][]Order, error) {
	rows, err := db.Query(ctx, ordersByCustomersSQL, customerIDs)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]Order)
	for rows.Next() {
		var order Order
		if err := rows.Scan(&order.CustomerID, &order.ID, &order.Amount); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		out[order.CustomerID] = append(out[order.CustomerID], order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orders: %w", err)
	}
	return out, nil
}

func CreateOrder(ctx context.Context, db DB, order Order) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, insertOrderSQL, order.ID, order.CustomerID, order.Amount); err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	if _, err := tx.Exec(ctx, insertAuditSQL, order.ID, "created"); err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func BulkAudit(ctx context.Context, db DB, entries []AuditEntry) (int64, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	source := pgx.CopyFromSlice(len(entries), func(i int) ([]any, error) {
		return []any{entries[i].OrderID, entries[i].Action}, nil
	})
	n, err := db.CopyFrom(ctx, pgx.Identifier{"audit_log"}, []string{"order_id", "action"}, source)
	if err != nil {
		return 0, fmt.Errorf("copy audit: %w", err)
	}
	return n, nil
}

func flatten(grouped map[string][]Order) []string {
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []string
	for _, key := range keys {
		for _, order := range grouped[key] {
			out = append(out, order.ID)
		}
	}
	return out
}

func main() {
	lab := labkit.New("Day 20", "pgx：批量查询、事务与 CopyFrom")
	lab.Section("框架简介：pgx")
	lab.Infof("类型：PostgreSQL 驱动与工具包")
	lab.Infof("是什么：原生 PostgreSQL 驱动，提供连接池、事务、批量查询、CopyFrom 和 PostgreSQL 类型支持")
	lab.Infof("解决什么问题：database/sql 的通用抽象会损失 PostgreSQL 特性；N+1 查询和逐行写入也不适合大数据量")
	lab.Infof("核心抽象：pgxpool.Pool、Begin/Tx、Query/Rows、CopyFrom、CollectRows、PostgreSQL 错误码")
	lab.Infof("采用情况：约 14k star，是 Go 连接 PostgreSQL 的事实标准之一；高吞吐服务普遍优先选择")
	lab.Infof("适合：PostgreSQL 专属功能、高吞吐、批量导入、JSONB/数组、LISTEN/NOTIFY")
	lab.Infof("慎用：pgx 绑定 PostgreSQL，不能无成本切数据库；CopyFrom 只适合批量导入，不替代事务和幂等设计")
	lab.Infof("对照：lib/pq 已进入维护状态；database/sql 更通用；GORM 可接 pgx，但复杂 SQL 仍应显式写")
	ctx := context.Background()
	db, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	if err != nil {
		lab.Check(false, "create pgx pool: %v", err)
		lab.Done()
		return
	}
	defer db.Close()

	lab.Section("场景 A：一次 ANY($1) 查询替代 N+1")
	rows := db.NewRows([]string{"customer_id", "id", "amount"}).
		AddRow("c-1", "o-1", int64(100)).
		AddRow("c-1", "o-2", int64(200)).
		AddRow("c-2", "o-3", int64(300))
	db.ExpectQuery(ordersByCustomersSQL).WithArgs(pgxmock.AnyArg()).WillReturnRows(rows)
	orders, err := LoadOrders(ctx, db, []string{"c-1", "c-2"})
	lab.Infof("orders=%v err=%v", flatten(orders), err)
	lab.Check(err == nil && len(orders["c-1"]) == 2 && len(orders["c-2"]) == 1, "一次往返拿回所有客户订单")
	lab.Check(len(flatten(orders)) == 3, "结果按客户和时间稳定排序")

	lab.Section("场景 B：订单和审计同一事务")
	db.ExpectBegin()
	db.ExpectExec(insertOrderSQL).
		WithArgs("o-9", "c-1", int64(990)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec(insertAuditSQL).
		WithArgs("o-9", "created").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()
	err = CreateOrder(ctx, db, Order{ID: "o-9", CustomerID: "c-1", Amount: 990})
	lab.Infof("create err=%v", err)
	lab.Check(err == nil, "Begin -> INSERT -> INSERT -> Commit")

	lab.Section("场景 C：审计失败时回滚")
	db.ExpectBegin()
	db.ExpectExec(insertOrderSQL).
		WithArgs("o-10", "c-2", int64(50)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec(insertAuditSQL).
		WithArgs("o-10", "created").
		WillReturnError(fmt.Errorf("audit unavailable"))
	db.ExpectRollback()
	err = CreateOrder(ctx, db, Order{ID: "o-10", CustomerID: "c-2", Amount: 50})
	lab.Infof("rollback err=%v", err)
	lab.Check(err != nil, "任一写入失败，事务回滚")

	lab.Section("场景 D：CopyFrom 批量写入")
	db.ExpectCopyFrom(pgx.Identifier{"audit_log"}, []string{"order_id", "action"}).WillReturnResult(150)
	n, err := BulkAudit(ctx, db, []AuditEntry{{OrderID: "o-1", Action: "created"}, {OrderID: "o-2", Action: "created"}})
	lab.Infof("copied=%d err=%v", n, err)
	lab.Check(err == nil && n == 150, "CopyFrom 用一条协议完成批量导入")

	lab.Section("场景 E：真实连接")
	lab.Infof("生产替换：pgxpool.New(ctx, os.Getenv(\"DATABASE_URL\"))")
	lab.Infof("LoadOrders 使用 ANY($1)，避免按 customer 循环查询")
	lab.Infof("CopyFrom 适合审计/事件批量写入；事务内失败必须 Rollback")
	lab.Done()
}
