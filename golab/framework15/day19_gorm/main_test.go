package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seed(t *testing.T) (*gorm.DB, Account, Account) {
	t.Helper()
	db, err := OpenDB(t.Name())
	require.NoError(t, err)
	alice := Account{Owner: "alice", Balance: 1000}
	bob := Account{Owner: "bob", Balance: 0}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)
	return db, alice, bob
}

func TestTransferCommitsBothSides(t *testing.T) {
	db, alice, bob := seed(t)
	applied, err := Transfer(db, "k1", alice.ID, bob.ID, 300)
	require.NoError(t, err)
	require.True(t, applied)
	a, _ := balance(db, "alice")
	b, _ := balance(db, "bob")
	require.Equal(t, int64(700), a)
	require.Equal(t, int64(300), b)
}

func TestDuplicateIsNoOp(t *testing.T) {
	db, alice, bob := seed(t)
	_, err := Transfer(db, "same", alice.ID, bob.ID, 100)
	require.NoError(t, err)
	applied, err := Transfer(db, "same", alice.ID, bob.ID, 100)
	require.NoError(t, err)
	require.False(t, applied)
	a, _ := balance(db, "alice")
	require.Equal(t, int64(900), a)
}

func TestInsufficientRollsBackLedger(t *testing.T) {
	db, alice, bob := seed(t)
	_, err := Transfer(db, "too-much", alice.ID, bob.ID, 5000)
	require.True(t, errors.Is(err, ErrInsufficient))
	a, _ := balance(db, "alice")
	require.Equal(t, int64(1000), a)
	var entries int64
	require.NoError(t, db.Model(&LedgerEntry{}).Count(&entries).Error)
	require.Zero(t, entries)
}

func TestOptimisticVersionIncrements(t *testing.T) {
	db, alice, bob := seed(t)
	_, err := Transfer(db, "v1", alice.ID, bob.ID, 50)
	require.NoError(t, err)
	var got Account
	require.NoError(t, db.First(&got, alice.ID).Error)
	require.Equal(t, int64(1), got.Version)
}
