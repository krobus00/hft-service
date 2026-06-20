package constant

const (
	MessageSuccess = "success"

	SuccessStatusCode      = "SUCCESS"
	BadRequestStatusCode   = "BAD_REQUEST"
	UnauthorizedStatusCode = "UNAUTHORIZED"
	ForbiddenStatusCode    = "FORBIDDEN"
	NotFoundStatusCode     = "NOT_FOUND"
	ConflictStatusCode     = "CONFLICT"
	InternalStatusCode     = "INTERNAL_ERROR"
)

const (
	PermissionOrderRead           = "order:read"
	PermissionOrderWrite          = "order:write"
	PermissionOrderReportRead     = "order_report:read"
	PermissionMarketRead          = "market:read"
	PermissionMarketConfigWrite   = "market_config:write"
	PermissionStrategyConfigRead  = "strategy_config:read"
	PermissionStrategyConfigWrite = "strategy_config:write"
	PermissionSettingsRead        = "settings:read"
	PermissionSettingsWrite       = "settings:write"
	PermissionUserRead            = "user:read"
	PermissionUserWrite           = "user:write"
	PermissionDashboardPageRead   = "dashboard_page:read"
	PermissionDashboardPageWrite  = "dashboard_page:write"
	PermissionPermissionRead      = "permission:read"
	PermissionPermissionWrite     = "permission:write"
)
