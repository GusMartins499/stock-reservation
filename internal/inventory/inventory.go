package inventory

import (
	"errors"
	"time"
)

type Product struct {
	SKU   string
	Name  string
	Stock int
}

type ReservationStatus string

const (
	StatusPending   ReservationStatus = "pending"
	StatusConfirmed ReservationStatus = "confirmed"
	StatusCancelled ReservationStatus = "cancelled"
	StatusExpired   ReservationStatus = "expired"
)

type Reservation struct {
	ID        string
	SKU       string
	Quantity  int
	Status    ReservationStatus
	ExpiresAt time.Time
	CreatedAt time.Time
}

var (
	ErrProductNotFound       = errors.New("product not found")
	ErrProductAlreadyExists  = errors.New("product already exists")
	ErrInsufficientStock     = errors.New("insufficient stock")
	ErrInvalidQuantity       = errors.New("invalid quantity")
	ErrInvalidSKU            = errors.New("invalid SKU")
	ErrReservationNotFound   = errors.New("reservation not found")
	ErrReservationNotPending = errors.New("reservation is not pending")
	ErrReservationExpired    = errors.New("reservation expired")
)
