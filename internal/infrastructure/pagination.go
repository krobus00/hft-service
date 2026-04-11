package infrastructure

import (
	"net/http"
	"strconv"
	"strings"
)

const (
	defaultPage     = 1
	defaultPageSize = 10
	maxPageSize     = 100
)

type PaginationRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

type PaginationResponse struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

type PaginatedData[T any] struct {
	Items      []T                `json:"items"`
	Pagination PaginationResponse `json:"pagination"`
}

func ParsePaginationRequest(r *http.Request) PaginationRequest {
	page := parsePositiveInt(r.URL.Query().Get("page"), defaultPage)
	pageSize := parsePositiveInt(r.URL.Query().Get("page_size"), defaultPageSize)

	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	return PaginationRequest{
		Page:     page,
		PageSize: pageSize,
	}
}

func NewPaginationResponse(req PaginationRequest, totalItems int64) PaginationResponse {
	totalPages := int((totalItems + int64(req.PageSize) - 1) / int64(req.PageSize))
	if totalPages == 0 {
		totalPages = 1
	}

	if req.Page > totalPages {
		req.Page = totalPages
	}

	return PaginationResponse{
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
		HasNext:    req.Page < totalPages,
		HasPrev:    req.Page > 1,
	}
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}

	return value
}
