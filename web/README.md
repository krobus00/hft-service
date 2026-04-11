# Krobot Web

Next.js dashboard frontend for HFT-service.

## Requirements

- Node.js 20+
- Dashboard gateway service running (default: http://localhost:9803)

## Environment

Copy [.env.example](.env.example) and adjust values if needed.

- DASHBOARD_GATEWAY_URL: dashboard-gateway base URL used by Next API routes

## Local Run

1. Install dependencies:
	npm install
2. Start development server:
	npm run dev
3. Open:
	http://localhost:3000

## Auth Flow

- Login page sends username/password to [src/app/api/auth/login/route.ts](src/app/api/auth/login/route.ts).
- Next API routes call dashboard-gateway endpoints:
  - POST /dashboard/v1/auth/login
  - POST /dashboard/v1/auth/refresh
  - GET /dashboard/v1/auth/me
- Tokens are stored as httpOnly cookies.
- Middleware guards dashboard routes in [middleware.ts](middleware.ts).

## User Management API

Dashboard user management is proxied through Next API routes:

- GET [src/app/api/dashboard/users/route.ts](src/app/api/dashboard/users/route.ts)
- POST [src/app/api/dashboard/users/route.ts](src/app/api/dashboard/users/route.ts)
- GET [src/app/api/dashboard/users/[id]/route.ts](src/app/api/dashboard/users/[id]/route.ts)
- PATCH [src/app/api/dashboard/users/[id]/route.ts](src/app/api/dashboard/users/[id]/route.ts)
- DELETE [src/app/api/dashboard/users/[id]/route.ts](src/app/api/dashboard/users/[id]/route.ts)

These call dashboard-gateway endpoints under `/dashboard/v1/users`.

## Pagination Standard

Pagination request query for list endpoints:

- `page` (int, starts at 1)
- `page_size` (int, max 100)
- `search` (optional)

Standard paginated response shape:

- `data.items`: list payload
- `data.pagination.page`
- `data.pagination.page_size`
- `data.pagination.total_items`
- `data.pagination.total_pages`
- `data.pagination.has_next`
- `data.pagination.has_prev`

## Design System

Follow [DESIGN_GUIDE.md](DESIGN_GUIDE.md) for all new screens.

## Main App Structure

- App routes: [src/app](src/app)
- API route handlers: [src/app/api](src/app/api)
- Shared components: [src/components](src/components)
- Server utilities: [src/lib/server](src/lib/server)
- Types: [src/lib/types](src/lib/types)
