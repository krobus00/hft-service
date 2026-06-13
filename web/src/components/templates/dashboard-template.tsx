import { BrandWordmark } from "@/components/atoms/brand-wordmark";
import { Button } from "@/components/ui/button";
import type { AuthUser } from "@/types/auth";

type DashboardTemplateProps = {
  children: React.ReactNode;
  user: AuthUser;
  onLogout: () => void;
};

export function DashboardTemplate({ children, user, onLogout }: DashboardTemplateProps) {
  return (
    <main className="min-h-screen bg-background">
      <header className="border-b bg-card">
        <div className="mx-auto flex h-16 w-full max-w-7xl items-center justify-between px-6">
          <BrandWordmark />
          <div className="flex items-center gap-3">
            <div className="hidden text-right text-sm sm:block">
              <p className="font-medium leading-5">{user.name || "User"}</p>
              <p className="text-xs text-muted-foreground">{user.email}</p>
            </div>
            <Button variant="outline" size="sm" onClick={onLogout}>
              Logout
            </Button>
          </div>
        </div>
      </header>
      <div className="mx-auto w-full max-w-7xl px-6 py-8">{children}</div>
    </main>
  );
}
