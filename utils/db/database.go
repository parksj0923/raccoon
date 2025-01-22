package db

import (
	"context"
	"database/sql"
	"fmt"
)

type Database struct {
	DbForJet *sql.DB
}

type TransactionChain[T any] struct {
	block           func(ctx context.Context) (error, T)
	failedCallBack  func(err error) (error, T)
	finallyCallBack func()
}

func Transaction[T any](block func(ctx context.Context) (error, T)) *TransactionChain[T] {
	return &TransactionChain[T]{
		block: block,
	}
}

func (transaction *TransactionChain[T]) Failed(failedCallBack func(err error) (error, T)) *TransactionChain[T] {
	transaction.failedCallBack = failedCallBack
	return transaction
}

func (transaction *TransactionChain[T]) Finally(finallyCallBack func()) *TransactionChain[T] {
	transaction.finallyCallBack = finallyCallBack
	return transaction
}

func (transaction *TransactionChain[T]) Run(ctx context.Context, db *sql.DB) (error, T) {
	tx, err := db.Begin()
	ctx = context.WithValue(ctx, "tx", tx)
	if err != nil {
		_ = fmt.Errorf("transaction start failed")
	}

	defer func() {
		if r := recover(); r != nil {
			if txErr := tx.Rollback(); txErr != nil {
				_ = fmt.Errorf("transaction rollback failed")
			}
			if transaction.failedCallBack != nil {
				var finalErr error
				if err, ok := r.(error); ok {
					finalErr = err
				} else {
					panic(r)
				}
				_, _ = transaction.failedCallBack(finalErr)
			}
		}
		if transaction.finallyCallBack != nil {
			transaction.finallyCallBack()
		}
	}()
	err, results := transaction.block(ctx)
	if err != nil {
		if txErr := tx.Rollback(); txErr != nil {
			_ = fmt.Errorf("transaction rollback failed")
		}
		if transaction.failedCallBack != nil {
			return transaction.failedCallBack(err)
		}
	}

	if err := tx.Commit(); err != nil {
		_ = fmt.Errorf("transaction commit failed")
	}
	if transaction.finallyCallBack != nil {
		transaction.finallyCallBack()
	}
	return err, results
}
