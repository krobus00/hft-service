# Krobot Web Design Guide

This guide defines the baseline style and architecture for all pages in this Next.js app.

## 1. Design Principles

- Keep pages minimalist, data-first, and high contrast.
- Use one accent color family (blue scale) for actions and highlights.
- Prefer flat surfaces with subtle borders and very light shadows.
- Keep spacing consistent using an 8px rhythm.
- UI should feel operational and calm, not decorative.

## 2. Color System

All tokens live in [src/app/globals.css](src/app/globals.css).

- Background: --background (#edf2fb)
- Surface: --surface (#ffffff)
- Border: --line (#d7e0ef)
- Text main: --foreground (#0f172a)
- Text secondary: --text-muted (#64748b)
- Primary action: --brand (#2563eb)
- Primary hover: --brand-strong (#1d4ed8)
- Error: --danger (#d63649)

Do not hardcode arbitrary colors in components unless there is a one-off need.

## 3. Typography

- Primary font: Geist Sans via [src/app/layout.tsx](src/app/layout.tsx).
- Eyebrow text: uppercase, 12px, muted color.
- Heading scale:
	- H1: 30-36px
	- H2: 22px
	- Body: 14-16px

## 4. Layout Rules

- App shell uses a left sidebar and right content area.
- Header always includes page title on left and user section on right.
- Main content is card-based and uses reusable flat panel style.
- Mobile breakpoint is 980px where sidebar stacks above content.

## 5. Components To Reuse

- Login form: [src/components/auth/login-form.tsx](src/components/auth/login-form.tsx)
- Logout action: [src/components/auth/logout-button.tsx](src/components/auth/logout-button.tsx)
- Sidebar nav: [src/components/dashboard/sidebar.tsx](src/components/dashboard/sidebar.tsx)
- User chip: [src/components/dashboard/user-chip.tsx](src/components/dashboard/user-chip.tsx)

When adding new pages, extend these components first before creating new patterns.

## 5A. Minimal Styling Rules

- Avoid gradients for cards, navigation, and containers.
- Keep radius modest (8px to 14px range).
- Use transitions sparingly (hover/focus only).
- Avoid oversized shadows and unnecessary decoration.

## 6. States And Feedback

- Loading states should show concise inline text.
- Errors should use --danger color and clear actionable copy.
- Disabled buttons should reduce opacity and use not-allowed cursor.

## 7. Next.js Architecture Standard

- Keep feature logic in [src/lib](src/lib):
	- Types in [src/lib/types](src/lib/types)
	- Server-only gateway and cookie utilities in [src/lib/server](src/lib/server)
- Keep browser pages thin by calling local API routes in [src/app/api](src/app/api).
- Use middleware in [middleware.ts](middleware.ts) for route protection.
- Keep auth tokens in httpOnly cookies only.

## 8. Naming Convention

- Use clear file names by feature and purpose (example: user-chip, login-form).
- Keep CSS class names semantic and reusable (panel, stat-card, user-chip).
- Prefer explicit type names over generic aliases.

## 9. Implementation Checklist For New Screens

1. Use existing color and spacing tokens.
2. Reuse panel and header patterns.
3. Keep business logic in route handlers and lib/server.
4. Add loading and error states.
5. Verify desktop and mobile behavior.
