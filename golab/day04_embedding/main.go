package main

import (
	"errors"
	"strings"

	"golab/internal/labkit"
)

type BaseExecutor struct {
	Name string
}

func (b *BaseExecutor) ID() string { return b.Name }

func (b *BaseExecutor) Run() string { return "run(" + b.ID() + ")" }

func (b *BaseExecutor) Validate() error {
	if strings.TrimSpace(b.Name) == "" {
		return errors.New("executor name is required")
	}
	return nil
}

// SQLExecutor gets Run and Validate by embedding. It overrides ID, but Run is
// still BaseExecutor.Run, so b.ID() is statically bound to BaseExecutor.ID.
// This is the classic Go trap: embedding is composition, not virtual dispatch.
type SQLExecutor struct {
	*BaseExecutor
	SQL string
}

func (s *SQLExecutor) ID() string { return "sql:" + s.BaseExecutor.Name }

// FixedSQLExecutor explicitly composes the behavior it wants.
type FixedSQLExecutor struct {
	*BaseExecutor
	SQL string
}

func (s *FixedSQLExecutor) ID() string  { return "sql:" + s.BaseExecutor.Name }
func (s *FixedSQLExecutor) Run() string { return "run(" + s.ID() + ")" }

// Named is a capability. Embedding the interface (not the concrete type) keeps
// the wrapper's public method set small.
type Named interface {
	ID() string
}

type ReadOnlyExecutor struct {
	Named
}

// Counter has a pointer receiver, so *Counter implements Incer but Counter does
// not. This is why method sets matter when passing values to interfaces.
type Counter struct{ N int }

func (c *Counter) Inc() { c.N++ }

type Incer interface{ Inc() }

var _ Incer = (*Counter)(nil)

func main() {
	lab := labkit.New("Day 04", "embedding：组合不是继承")

	lab.Section("场景 A：嵌入后的方法提升")
	sql := &SQLExecutor{BaseExecutor: &BaseExecutor{Name: "orders_sync"}, SQL: "select 1"}
	lab.Infof("sql.ID()=%q", sql.ID())
	lab.Infof("sql.Run()=%q", sql.Run())
	lab.Check(sql.ID() == "sql:orders_sync", "外层 ID 方法被提升到 SQLExecutor")
	lab.Check(sql.Run() == "run(orders_sync)", "Run 内部调用的是 BaseExecutor.ID，而不是外层 ID")
	lab.Check(sql.Validate() == nil, "Validate 也被提升，可直接复用")

	lab.Section("场景 B：显式组合修复行为")
	fixed := &FixedSQLExecutor{BaseExecutor: &BaseExecutor{Name: "orders_sync"}, SQL: "select 1"}
	lab.Infof("fixed.Run()=%q", fixed.Run())
	lab.Check(fixed.Run() == "run(sql:orders_sync)", "显式实现 Run 后，行为符合多态预期")

	lab.Section("场景 C：嵌入接口限制暴露面")
	ro := ReadOnlyExecutor{Named: fixed}
	lab.Infof("ReadOnlyExecutor 只能看到 ID()，看不到 Run/Validate：%q", ro.ID())
	lab.Check(ro.ID() == "sql:orders_sync", "嵌入接口可用于收窄 API")

	lab.Section("场景 D：方法集与指针接收者")
	c := &Counter{}
	c.Inc()
	c.Inc()
	lab.Infof("counter=%d", c.N)
	lab.Check(c.N == 2, "*Counter 实现 Incer；Counter 值本身不实现")

	lab.Section("场景 E：空名字校验")
	err := (&SQLExecutor{BaseExecutor: &BaseExecutor{}, SQL: "x"}).Validate()
	lab.Infof("validate error = %v", err)
	lab.Check(err != nil, "提升的 Validate 仍然可用，避免每个执行器重复写校验")

	lab.Section("Java → Go 对照")
	lab.Infof("extends BaseExecutor          ≈  嵌入 *BaseExecutor（只有方法提升）")
	lab.Infof("父类 this.id() 虚分派          ≈  Go 没有虚分派；需要显式实现外层方法")
	lab.Infof("implements ReadOnly           ≈  嵌入小接口，主动收窄方法集")
	lab.Infof("对象方法在堆上                 ≈  值/指针接收者决定方法集，接口赋值前先想清楚")
	lab.Done()
}
