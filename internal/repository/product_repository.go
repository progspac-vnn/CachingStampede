// Package repository provides storage access for domain models. Repositories
// talk only to storage — they contain no business logic and no HTTP
// concerns.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/progspac-vnn/CachingStampede/internal/model"
)

// ErrProductNotFound is returned when a requested product does not exist.
var ErrProductNotFound = errors.New("repository: product not found")

// ProductRepository provides read access to products stored in PostgreSQL.
type ProductRepository struct {
	pool *pgxpool.Pool
}

// NewProductRepository constructs a ProductRepository backed by the given
// connection pool.
func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

// GetByID fetches a single product by its ID. It returns ErrProductNotFound
// if no matching row exists.
func (r *ProductRepository) GetByID(ctx context.Context, id int64) (*model.Product, error) {
	const query = `
		SELECT id, name, description, price::float8, stock, created_at, updated_at
		FROM products
		WHERE id = $1
	`

	var p model.Product
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, fmt.Errorf("repository: failed to get product %d: %w", id, err)
	}

	return &p, nil
}

// List returns all products ordered by ID.
func (r *ProductRepository) List(ctx context.Context) ([]*model.Product, error) {
	const query = `
		SELECT id, name, description, price::float8, stock, created_at, updated_at
		FROM products
		ORDER BY id
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("repository: failed to list products: %w", err)
	}
	defer rows.Close()

	var products []*model.Product
	for rows.Next() {
		var p model.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("repository: failed to scan product row: %w", err)
		}
		products = append(products, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: error iterating product rows: %w", err)
	}

	return products, nil
}
