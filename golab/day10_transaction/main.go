package main

import (
	"errors"
	"fmt"

	"golab/internal/labkit"
	dbtx "golab/internal/tx"
)

func rows(n int) []dbtx.Row {
	out := make([]dbtx.Row, n)
	for i := range out {
		out[i] = dbtx.Row{"name": fmt.Sprintf("row-%03d", i)}
	}
	return out
}

func main() {
	lab := labkit.New("Day 10", "事务：显式边界、回滚、快照与批量写入")

	lab.Section("场景 A：100 行数据，N+1 vs 批量")
	n1 := dbtx.NewDB()
	if err := n1.InsertOneByOne("orders", rows(100)); err != nil {
		lab.Failf("n+1 insert: %v", err)
	}
	batch := dbtx.NewDB()
	if err := batch.BatchInsert("orders", rows(100)); err != nil {
		lab.Failf("batch insert: %v", err)
	}
	n1s, bs := n1.Stats(), batch.Stats()
	lab.Infof("N+1   statements=%d transactions=%d commits=%d", n1s.Statements, n1s.Transactions, n1s.Commits)
	lab.Infof("batch statements=%d transactions=%d commits=%d", bs.Statements, bs.Transactions, bs.Commits)
	lab.Check(n1s.Statements == 100 && bs.Statements == 1, "批量写入把 100 条 SQL 压成 1 条")
	lab.Check(len(batch.Rows("orders")) == 100, "批量写入结果完整")

	lab.Section("场景 B：中途校验失败，整个事务回滚")
	db := dbtx.NewDB()
	tx := db.Begin()
	_ = tx.Insert("orders", dbtx.Row{"name": "ok"})
	err := tx.Insert("orders", dbtx.Row{"name": ""})
	lab.Infof("second insert error = %v", err)
	tx.Rollback()
	stats := db.Stats()
	lab.Infof("rows=%d rollbacks=%d", len(db.Rows("orders")), stats.Rollbacks)
	lab.Check(errors.Is(err, dbtx.ErrNameRequired), "业务校验错误可被 errors.Is 识别")
	lab.Check(len(db.Rows("orders")) == 0, "第一条已插入的记录也被回滚")

	lab.Section("场景 C：快照隔离")
	db2 := dbtx.NewDB()
	t1 := db2.Begin()
	t2 := db2.Begin()
	_ = t2.Insert("orders", dbtx.Row{"name": "from-t2"})
	_ = t2.Commit()
	lab.Infof("t1 sees %d rows, db sees %d rows", len(t1.Select("orders")), len(db2.Rows("orders")))
	lab.Check(len(t1.Select("orders")) == 0, "t1 的快照看不到 t2 后提交的数据")
	lab.Check(len(db2.Rows("orders")) == 1, "t2 已提交，其他新事务可见")
	t1.Rollback()

	lab.Section("场景 D：事务内可见自己的写")
	db3 := dbtx.NewDB()
	t3 := db3.Begin()
	_ = t3.Insert("orders", dbtx.Row{"name": "mine"})
	lab.Infof("inside tx rows=%d, outside rows=%d", len(t3.Select("orders")), len(db3.Rows("orders")))
	lab.Check(len(t3.Select("orders")) == 1 && len(db3.Rows("orders")) == 0, "未提交前只有事务自己看得到")
	_ = t3.Commit()
	lab.Check(len(db3.Rows("orders")) == 1, "提交后对外可见")

	lab.Section("Java → Go 对照")
	lab.Infof("@Transactional 代理            ≈  显式 tx := db.Begin(); defer rollback; Commit")
	lab.Infof("MyBatis batch                  ≈  BatchInsert，一次网络往返")
	lab.Infof("JPA 一级缓存                   ≈  事务内可见自己的写；Go 里由 store 显式实现")
	lab.Infof("隔离级别                        ≈  这里演示 snapshot read；生产要按 DB 方言设置")
	lab.Done()
}
