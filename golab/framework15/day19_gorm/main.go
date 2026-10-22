package main

import (
	"errors"
	"fmt"
	"strings"

	"frameworklab/internal/labkit"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ErrInsufficient    = errors.New("insufficient balance")
	ErrVersionConflict = errors.New("account version conflict")
	errDuplicate       = errors.New("duplicate idempotency key")
)

type Account struct {
	ID      uint   `gorm:"primaryKey"`
	Owner   string `gorm:"uniqueIndex;not null"`
	Balance int64  `gorm:"not null"`
	Version int64  `gorm:"not null;default:0"`
}

type LedgerEntry struct {
	ID             uint   `gorm:"primaryKey"`
	IdempotencyKey string `gorm:"uniqueIndex;not null"`
	FromAccountID  uint   `gorm:"not null"`
	ToAccountID    uint   `gorm:"not null"`
	Amount         int64  `gorm:"not null"`
}

func OpenDB(name string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&Account{}, &LedgerEntry{}); err != nil {
		return nil, err
	}
	return db, nil
}

// Transfer is idempotent: a repeated key is a successful no-op. Both account
// updates use the version column as an optimistic lock; a stale write aborts
// the transaction instead of overwriting another transaction.
func Transfer(db *gorm.DB, key string, fromID, toID uint, amount int64) (bool, error) {
	if amount <= 0 {
		return false, fmt.Errorf("amount must be positive")
	}
	applied := false
	err := db.Transaction(func(tx *gorm.DB) error {
		entry := LedgerEntry{IdempotencyKey: key, FromAccountID: fromID, ToAccountID: toID, Amount: amount}
		if err := tx.Create(&entry).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) || isUniqueViolation(err) {
				return errDuplicate
			}
			return fmt.Errorf("insert ledger: %w", err)
		}

		var from, to Account
		if err := tx.First(&from, fromID).Error; err != nil {
			return fmt.Errorf("load source account: %w", err)
		}
		if from.Balance < amount {
			return ErrInsufficient
		}
		if err := tx.First(&to, toID).Error; err != nil {
			return fmt.Errorf("load target account: %w", err)
		}

		debit := tx.Model(&Account{}).
			Where("id = ? AND version = ?", from.ID, from.Version).
			Updates(map[string]any{"balance": gorm.Expr("balance - ?", amount), "version": gorm.Expr("version + 1")})
		if debit.Error != nil {
			return fmt.Errorf("debit: %w", debit.Error)
		}
		if debit.RowsAffected != 1 {
			return ErrVersionConflict
		}

		credit := tx.Model(&Account{}).
			Where("id = ? AND version = ?", to.ID, to.Version).
			Updates(map[string]any{"balance": gorm.Expr("balance + ?", amount), "version": gorm.Expr("version + 1")})
		if credit.Error != nil {
			return fmt.Errorf("credit: %w", credit.Error)
		}
		if credit.RowsAffected != 1 {
			return ErrVersionConflict
		}
		applied = true
		return nil
	})
	if errors.Is(err, errDuplicate) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return applied, nil
}

func isUniqueViolation(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func balance(db *gorm.DB, owner string) (int64, error) {
	var account Account
	if err := db.Where("owner = ?", owner).First(&account).Error; err != nil {
		return 0, err
	}
	return account.Balance, nil
}

func main() {
	lab := labkit.New("Day 19", "GORM：事务、幂等键与乐观锁")
	lab.Section("框架简介：GORM")
	lab.Infof("类型：ORM")
	lab.Infof("是什么：Go 最常用的 ORM，提供模型映射、关联、事务、Hook 和多数据库方言")
	lab.Infof("解决什么问题：常规 CRUD 和关联加载如果全部手写 SQL，样板代码多且容易漏掉事务边界")
	lab.Infof("核心抽象：DB/Model、Where、Transaction、AutoMigrate、Expr、RowsAffected、方言 Dialector")
	lab.Infof("采用情况：约 40k star，Go 生态最常用的 ORM 之一；高性能和复杂 SQL 场景经常转向 pgx/sqlx")
	lab.Infof("适合：CRUD 为主、团队希望统一数据访问方式、查询复杂度可控的业务服务")
	lab.Infof("慎用：AutoMigrate 不是生产迁移方案；注意 N+1、Save 覆盖、零值更新、事务边界和隐式查询")
	lab.Infof("对照：pgx 更贴近 PostgreSQL；sqlx 更轻；ent 提供 schema 和代码生成；复杂报表仍应手写 SQL")
	db, err := OpenDB("day19")
	if err != nil {
		lab.Check(false, "open db: %v", err)
		lab.Done()
		return
	}
	alice := Account{Owner: "alice", Balance: 1000}
	bob := Account{Owner: "bob", Balance: 0}
	if err := db.Create(&alice).Error; err != nil {
		lab.Check(false, "seed alice: %v", err)
	}
	if err := db.Create(&bob).Error; err != nil {
		lab.Check(false, "seed bob: %v", err)
	}

	lab.Section("场景 A：转账在一个事务内完成")
	applied, err := Transfer(db, "transfer-001", alice.ID, bob.ID, 300)
	aliceBalance, _ := balance(db, "alice")
	bobBalance, _ := balance(db, "bob")
	lab.Infof("applied=%v err=%v alice=%d bob=%d", applied, err, aliceBalance, bobBalance)
	lab.Check(err == nil && applied, "首次转账成功")
	lab.Check(aliceBalance == 700 && bobBalance == 300, "借贷两侧同时提交")

	lab.Section("场景 B：重复幂等键是成功 no-op")
	applied, err = Transfer(db, "transfer-001", alice.ID, bob.ID, 300)
	var entries int64
	_ = db.Model(&LedgerEntry{}).Count(&entries).Error
	lab.Infof("applied=%v err=%v entries=%d", applied, err, entries)
	lab.Check(err == nil && !applied, "重复请求不报错，也不会重复扣款")
	lab.Check(entries == 1, "唯一索引挡住重复流水")

	lab.Section("场景 C：余额不足时整笔事务回滚")
	applied, err = Transfer(db, "transfer-002", alice.ID, bob.ID, 5000)
	aliceBalance, _ = balance(db, "alice")
	lab.Infof("applied=%v err=%v alice=%d", applied, err, aliceBalance)
	lab.Check(!applied && errors.Is(err, ErrInsufficient), "业务错误向上返回")
	lab.Check(aliceBalance == 700, "流水和余额都没有留下半成品")

	lab.Section("场景 D：方言与生产连接")
	lab.Infof("dialect=%s", db.Dialector.Name())
	lab.Infof("本实验用纯 Go SQLite 便于离线运行；生产可换成 gorm.io/driver/postgres + pgx")
	lab.Check(db.Dialector.Name() == "sqlite", "同一套 repository 代码可替换方言")
	lab.Done()
}
