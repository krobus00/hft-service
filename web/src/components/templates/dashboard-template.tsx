"use client";

import { LogOut, Menu, PanelLeftClose, PanelLeftOpen, Settings, User, X } from "lucide-react";
import { ReactNode, useState } from "react";

import { BrandWordmark } from "@/components/atoms/brand-wordmark";
import { ProfileSettingsModal } from "@/components/molecules/profile-settings-modal";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { AuthUser } from "@/types/auth";

type DashboardTemplateProps = {
  children: ReactNode;
  sidebar: (props: { collapsed: boolean; onNavigate?: () => void }) => ReactNode;
  pageTitle: string;
  pageDescription: string;
  user: AuthUser;
  onLogout: () => void;
  onUserChange: (user: AuthUser) => void;
};

export function DashboardTemplate({
  children,
  sidebar,
  pageTitle,
  pageDescription,
  user,
  onLogout,
  onUserChange,
}: DashboardTemplateProps) {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const [isMobileOpen, setIsMobileOpen] = useState(false);
  const [isUserMenuOpen, setIsUserMenuOpen] = useState(false);
  const [isProfileOpen, setIsProfileOpen] = useState(false);

  return (
    <main className="min-h-screen bg-background">
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-40 hidden border-r bg-card transition-[width] duration-200 lg:flex lg:flex-col",
          isCollapsed ? "w-20" : "w-64",
        )}
      >
        <div className="flex h-16 items-center justify-between border-b px-4">
          <div className={cn(isCollapsed && "sr-only")}>
            <BrandWordmark />
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={() => setIsCollapsed((current) => !current)}
          >
            {isCollapsed ? (
              <PanelLeftOpen className="h-4 w-4" />
            ) : (
              <PanelLeftClose className="h-4 w-4" />
            )}
          </Button>
        </div>
        <div className="flex-1 overflow-y-auto p-3">{sidebar({ collapsed: isCollapsed })}</div>
        <div className="border-t p-3">
          <UserBlock collapsed={isCollapsed} user={user} />
          <Button
            type="button"
            variant="outline"
            size="sm"
            className={cn("mt-3 w-full", isCollapsed && "px-0")}
            onClick={onLogout}
          >
            <LogOut className="h-4 w-4" />
            <span className={cn(isCollapsed && "sr-only")}>Logout</span>
          </Button>
        </div>
      </aside>

      {isMobileOpen ? (
        <div className="fixed inset-0 z-50 lg:hidden">
          <button
            type="button"
            className="absolute inset-0 bg-foreground/30"
            aria-label="Close menu"
            onClick={() => setIsMobileOpen(false)}
          />
          <aside className="relative flex h-full w-72 flex-col border-r bg-card shadow-lg">
            <div className="flex h-16 items-center justify-between border-b px-4">
              <BrandWordmark />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => setIsMobileOpen(false)}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
            <div className="flex-1 overflow-y-auto p-3">
              {sidebar({ collapsed: false, onNavigate: () => setIsMobileOpen(false) })}
            </div>
            <div className="border-t p-3">
              <UserBlock collapsed={false} user={user} />
              <Button type="button" variant="outline" size="sm" className="mt-3 w-full" onClick={onLogout}>
                <LogOut className="h-4 w-4" />
                Logout
              </Button>
            </div>
          </aside>
        </div>
      ) : null}

      <div className={cn("min-h-screen transition-[padding] duration-200", isCollapsed ? "lg:pl-20" : "lg:pl-64")}>
        <header className="sticky top-0 z-30 border-b bg-card/95 backdrop-blur">
          <div className="flex h-16 items-center justify-between px-4 sm:px-6">
            <div className="flex min-w-0 items-center gap-3">
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="lg:hidden"
                onClick={() => setIsMobileOpen(true)}
              >
                <Menu className="h-5 w-5" />
              </Button>
              <div className="lg:hidden">
                <BrandWordmark />
              </div>
              <div className="hidden min-w-0 sm:block">
                <h1 className="truncate text-base font-semibold leading-5">{pageTitle}</h1>
                <p className="truncate text-xs text-muted-foreground">{pageDescription}</p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <div className="min-w-0 text-right sm:hidden">
                <h1 className="truncate text-sm font-semibold">{pageTitle}</h1>
              </div>
              <div className="relative">
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  className="h-9 w-9 rounded-full"
                  onClick={() => setIsUserMenuOpen((current) => !current)}
                  title="User menu"
                >
                  <span className="flex h-7 w-7 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
                    {(user.name || user.email || "U").slice(0, 1).toUpperCase()}
                  </span>
                </Button>
                {isUserMenuOpen ? (
                  <div className="absolute right-0 top-11 z-50 w-56 rounded-md border bg-card p-2 shadow-lg">
                    <div className="border-b px-2 py-2">
                      <p className="truncate text-sm font-medium">{user.name || "User"}</p>
                      <p className="truncate text-xs text-muted-foreground">{user.email}</p>
                    </div>
                    <button
                      type="button"
                      className="mt-2 flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-muted"
                      onClick={() => {
                        setIsUserMenuOpen(false);
                        setIsProfileOpen(true);
                      }}
                    >
                      <Settings className="h-4 w-4" />
                      Profile settings
                    </button>
                    <button
                      type="button"
                      className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm text-destructive hover:bg-muted"
                      onClick={onLogout}
                    >
                      <LogOut className="h-4 w-4" />
                      Logout
                    </button>
                  </div>
                ) : null}
              </div>
            </div>
          </div>
        </header>
        <div className="w-full px-4 py-6 sm:px-6 lg:px-8">{children}</div>
      </div>
      {isProfileOpen ? (
        <ProfileSettingsModal
          user={user}
          onSaved={onUserChange}
          onClose={() => setIsProfileOpen(false)}
        />
      ) : null}
    </main>
  );
}

function UserBlock({ collapsed, user }: { collapsed: boolean; user: AuthUser }) {
  return (
    <div className={cn("rounded-md border bg-background px-3 py-2", collapsed && "px-2 text-center")}>
      <p className={cn("truncate text-sm font-medium", collapsed && "sr-only")}>
        {user.name || "User"}
      </p>
      <p className={cn("truncate text-xs text-muted-foreground", collapsed && "sr-only")}>
        {user.email}
      </p>
      <div className={cn("mt-2 flex flex-wrap gap-1", collapsed && "hidden")}>
        {user.roles.map((role) => (
          <Badge key={role} variant="secondary" className="max-w-full truncate">
            {role}
          </Badge>
        ))}
      </div>
      {collapsed ? (
        <div className="mx-auto flex h-8 w-8 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground">
          <User className="h-4 w-4" />
        </div>
      ) : null}
    </div>
  );
}
