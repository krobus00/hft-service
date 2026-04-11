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

## Design System

Follow [DESIGN_GUIDE.md](DESIGN_GUIDE.md) for all new screens.

## Main App Structure

- App routes: [src/app](src/app)
- API route handlers: [src/app/api](src/app/api)
- Shared components: [src/components](src/components)
- Server utilities: [src/lib/server](src/lib/server)
- Types: [src/lib/types](src/lib/types)
