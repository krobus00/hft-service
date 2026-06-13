package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	sq "github.com/Masterminds/squirrel"
)

type PaginationEntity interface {
	GetColumn() map[string]string
	DefaultSort() SortReq
	SearchableFields() []string
}

type PaginateReq struct {
	Page   int64 `form:"page" json:"page"`
	Limit  int64 `form:"limit" json:"limit"`
	Offset int64 `form:"-" json:"-"`
}

type FilterReq struct {
	Field string   `form:"field" json:"field"`
	Op    string   `form:"op" json:"op"`
	Value []string `form:"value" json:"value"`
}

type SortReq struct {
	Field     string `form:"field" json:"field"`
	Direction string `form:"direction" json:"direction"`
}

type PaginationQueryParamReq struct {
	Keyword    string `form:"keyword" json:"keyword"`
	SearchBy   string `form:"searchBy" json:"searchBy"`
	Page       int64  `form:"page" json:"page"`
	Limit      int64  `form:"limit" json:"limit"`
	Pagination string `form:"pagination" json:"pagination"`
	Filter     string `form:"filter" json:"filter"`
	Sort       string `form:"sort" json:"sort"`
}

type PaginationReq struct {
	Keyword  string      `json:"keyword"`
	SearchBy string      `json:"searchBy"`
	Paginate PaginateReq `json:"paginate"`
	Filter   []FilterReq `json:"filter"`
	Sort     SortReq     `json:"sort"`
}

func NewPaginationQueryParamReq(values url.Values) *PaginationQueryParamReq {
	return &PaginationQueryParamReq{
		Keyword:    strings.TrimSpace(values.Get("keyword")),
		SearchBy:   strings.TrimSpace(values.Get("searchBy")),
		Pagination: strings.TrimSpace(values.Get("pagination")),
		Filter:     strings.TrimSpace(values.Get("filter")),
		Sort:       strings.TrimSpace(values.Get("sort")),
		Page:       parseInt64(values.Get("page")),
		Limit:      parseInt64(values.Get("limit")),
	}
}

func (r *PaginationReq) ApplyFilter(query sq.SelectBuilder, entity PaginationEntity) sq.SelectBuilder {
	for _, v := range r.Filter {
		if len(v.Value) == 0 {
			continue
		}

		tableColumn, exists := entity.GetColumn()[v.Field]
		if !exists {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(v.Op)) {
		case "eq":
			query = query.Where(sq.Eq{tableColumn: v.Value[0]})
		case "neq":
			query = query.Where(sq.NotEq{tableColumn: v.Value[0]})
		case "gt":
			query = query.Where(sq.Gt{tableColumn: v.Value[0]})
		case "gte":
			query = query.Where(sq.GtOrEq{tableColumn: v.Value[0]})
		case "lt":
			query = query.Where(sq.Lt{tableColumn: v.Value[0]})
		case "lte":
			query = query.Where(sq.LtOrEq{tableColumn: v.Value[0]})
		case "contain":
			query = query.Where(sq.Like{tableColumn: "%" + v.Value[0] + "%"})
		case "in":
			query = query.Where(sq.Eq{tableColumn: v.Value})
		case "between":
			if len(v.Value) >= 2 {
				query = query.Where(sq.And{sq.GtOrEq{tableColumn: v.Value[0]}, sq.LtOrEq{tableColumn: v.Value[1]}})
			}
		}
	}

	searchableFields := entity.SearchableFields()
	if r.Keyword != "" && len(searchableFields) > 0 {
		orExprs := make([]sq.Sqlizer, 0, len(searchableFields))
		for _, v := range searchableFields {
			orExprs = append(orExprs, sq.Like{fmt.Sprintf("LOWER(%s)", v): "%" + strings.ToLower(r.Keyword) + "%"})
		}
		query = query.Where(sq.Or(orExprs))
	}

	return query
}

func (r *PaginationReq) ParseFromQueryParam(qp *PaginationQueryParamReq, model PaginationEntity) error {
	if qp.Pagination != "" {
		rawPagination := decodeQueryJSONParam(qp.Pagination)
		if err := json.Unmarshal([]byte(rawPagination), &r.Paginate); err != nil {
			return err
		}
	} else {
		r.Paginate.Page = qp.Page
		r.Paginate.Limit = qp.Limit
	}

	if r.Paginate.Page == 0 && r.Paginate.Limit == 0 {
		r.Paginate.Page = 1
		r.Paginate.Limit = 10
	} else {
		if r.Paginate.Page <= 0 {
			r.Paginate.Page = 1
		}
		if r.Paginate.Limit <= 0 {
			r.Paginate.Limit = 10
		}
		if r.Paginate.Limit > 100 {
			r.Paginate.Limit = 100
		}
	}
	r.Paginate.Offset = (r.Paginate.Page - 1) * r.Paginate.Limit

	if qp.Filter != "" {
		filters := []FilterReq{}
		rawFilter := decodeQueryJSONParam(qp.Filter)
		if err := json.Unmarshal([]byte(rawFilter), &filters); err != nil {
			return err
		}

		for _, v := range filters {
			op := strings.ToLower(strings.TrimSpace(v.Op))
			switch op {
			case "eq", "neq", "in", "contain", "between", "gte", "lte", "gt", "lt":
				v.Op = op
				r.Filter = append(r.Filter, v)
			}
		}
	}

	if qp.Sort != "" {
		rawSort := decodeQueryJSONParam(qp.Sort)
		if err := json.Unmarshal([]byte(rawSort), &r.Sort); err != nil {
			return err
		}
	}

	r.Sort.Direction = strings.ToUpper(strings.TrimSpace(r.Sort.Direction))
	if r.Sort.Direction != "ASC" && r.Sort.Direction != "DESC" {
		r.Sort.Direction = "ASC"
	}

	if tableColumn, exists := model.GetColumn()[r.Sort.Field]; exists {
		r.Sort.Field = tableColumn
	}
	if r.Sort.Field == "" {
		r.Sort = model.DefaultSort()
	}

	r.Keyword = qp.Keyword
	r.SearchBy = strings.TrimSpace(qp.SearchBy)
	return nil
}

type PaginationMetaResp struct {
	Page       int64 `json:"page"`
	Limit      int64 `json:"limit"`
	TotalItems int64 `json:"totalItems"`
	TotalPages int64 `json:"totalPages"`
}

type PaginationResp struct {
	Meta  PaginationMetaResp `json:"meta"`
	Items any                `json:"items"`
}

func NewPaginationResp(page, limit, totalItems int64, items any) *PaginationResp {
	totalPages := int64(0)
	if limit > 0 {
		totalPages = totalItems / limit
		if totalItems%limit != 0 {
			totalPages++
		}
	}

	return &PaginationResp{
		Meta: PaginationMetaResp{
			Page:       page,
			Limit:      limit,
			TotalItems: totalItems,
			TotalPages: totalPages,
		},
		Items: items,
	}
}

func decodeQueryJSONParam(raw string) string {
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return raw
	}

	trimmedRaw := strings.TrimSpace(raw)
	trimmedDecoded := strings.TrimSpace(decoded)
	if strings.HasPrefix(trimmedRaw, "{") || strings.HasPrefix(trimmedRaw, "[") {
		return raw
	}
	if strings.HasPrefix(trimmedDecoded, "{") || strings.HasPrefix(trimmedDecoded, "[") {
		return decoded
	}

	return raw
}

func parseInt64(raw string) int64 {
	var value int64
	_, _ = fmt.Sscan(strings.TrimSpace(raw), &value)
	return value
}
