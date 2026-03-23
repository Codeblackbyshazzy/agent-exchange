package store

import (
	"context"

	"github.com/parlakisik/agent-exchange/aex-settlement/internal/model"
)

// SettlementStore defines the interface for settlement persistence
type SettlementStore interface {
	// Executions
	SaveExecution(ctx context.Context, execution model.Execution) error
	GetExecution(ctx context.Context, executionID string) (model.Execution, error)
	ListExecutionsByTenant(ctx context.Context, tenantID string, limit int) ([]model.Execution, error)
	ListExecutionsByContract(ctx context.Context, contractID string) (model.Execution, error)

	// Ledger
	AppendLedgerEntry(ctx context.Context, entry model.LedgerEntry) error
	GetLedgerEntries(ctx context.Context, tenantID string, limit int) ([]model.LedgerEntry, error)

	// Balances
	GetBalance(ctx context.Context, tenantID string) (model.TenantBalance, error)
	UpdateBalance(ctx context.Context, balance model.TenantBalance) error
	// IncrementBalance atomically increments (or decrements if negative) the balance
	// for the given tenant by deltaCents. It upserts the document if it does not exist.
	// Returns the updated TenantBalance after the increment.
	IncrementBalance(ctx context.Context, tenantID string, deltaCents int64, currency string) (model.TenantBalance, error)

	// Transactions
	SaveTransaction(ctx context.Context, tx model.Transaction) error
	GetTransaction(ctx context.Context, txID string) (model.Transaction, error)
	ListTransactions(ctx context.Context, tenantID string, limit int) ([]model.Transaction, error)

	// WithTransaction executes fn within a database transaction.
	// All store operations using the returned context participate in the transaction.
	// The transaction is committed if fn returns nil, rolled back otherwise.
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error

	Close() error
}
