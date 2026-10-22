package inventory

import (
	"errors"
	"strings"
)

type Product struct {
	SKU   string
	Name  string
	Stock int
}

var (
	ErrProductNotFound      = errors.New("product not found")
	ErrProductAlreadyExists = errors.New("product already exists")
	ErrInsufficientStock    = errors.New("insufficient stock")
	ErrInvalidQuantity      = errors.New("invalid quantity")
	ErrInvalidSKU           = errors.New("invalid SKU")
)

type Store struct {
	products map[string]*Product
}

func NewStore() *Store {
	return &Store{products: make(map[string]*Product)}
}

func (s *Store) GetProduct(sku string) (Product, error) {
	if p, exists := s.products[sku]; exists {
		return *p, nil
	}
	return Product{}, ErrProductNotFound
}

func (s *Store) Reserve(sku string, qty int) error {
	if qty <= 0 {
		return ErrInvalidQuantity
	}

	product, exists := s.products[sku]

	if !exists {
		return ErrProductNotFound
	}

	if product.Stock < qty {
		return ErrInsufficientStock
	}

	product.Stock -= qty

	return nil
}

func (s *Store) AddProduct(p Product) error {
	if p.Stock < 0 {
		return ErrInvalidQuantity
	}
	if strings.TrimSpace(p.SKU) == "" {
		return ErrInvalidSKU
	}
	if _, exists := s.products[p.SKU]; exists {
		return ErrProductAlreadyExists
	}
	s.products[p.SKU] = &p
	return nil
}
