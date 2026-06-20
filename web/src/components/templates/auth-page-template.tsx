import { BrandWordmark } from "@/components/atoms/brand-wordmark";
import { ThemeToggle } from "@/components/atoms/theme-toggle";
import { Badge } from "@/components/ui/badge";

type AuthPageTemplateProps = {
  children: React.ReactNode;
};

export function AuthPageTemplate({ children }: AuthPageTemplateProps) {
  return (
    <main className="min-h-screen bg-background">
      <div className="mx-auto flex min-h-screen w-full max-w-6xl flex-col px-6 py-8">
        <header className="flex items-center justify-between">
          <BrandWordmark />
          <div className="flex items-center gap-2">
            <Badge variant="success">Secure dashboard</Badge>
            <ThemeToggle />
          </div>
        </header>
        <section className="flex flex-1 items-center justify-center py-10">
          {children}
        </section>
      </div>
    </main>
  );
}
