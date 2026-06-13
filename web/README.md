# Krobot Web

Next.js TypeScript dashboard for the Krobot API service.

## Run Locally

```bash
cd web
npm install
npm run dev
```

PowerShell on this machine blocks `npm.ps1`; use `npm.cmd install` and `npm.cmd run dev` if needed.

Set the API base URL with:

```bash
NEXT_PUBLIC_API_BASE_URL=http://localhost:9804/api/v1
```

## Auth Routes

- `/setup` calls `POST /api/v1/auth/setup`
- `/login` calls `POST /api/v1/auth/login`
- `/` calls `GET /api/v1/auth/me`, refreshes with `POST /api/v1/auth/refresh`, and logs out with `POST /api/v1/auth/logout`

## Structure

Atomic design folders live under `src/components`:

- `atoms`
- `molecules`
- `organisms`
- `templates`
- `ui` for shadcn-compatible primitives

## Theme

Change colors and radius in `src/app/globals.css`. Tailwind tokens read those CSS variables through `tailwind.config.ts`.
