// Package model defines the domain types shared across the application's
// service and repository layers.
package model

import "time"

// Product represents a catalog item persisted in PostgreSQL.
type Product struct {
	ID          int64
	Name        string
	Description string
	Price       float64
	Stock       int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
