import { BrandWordmark } from "@/components/atoms/brand-wordmark";

type AuthPageTemplateProps = {
  children: React.ReactNode;
};

export function AuthPageTemplate({ children }: AuthPageTemplateProps) {
  return (
    <main className="min-h-screen bg-background">
      <div className="mx-auto flex min-h-screen w-full max-w-6xl flex-col px-6 py-8">
        <header className="flex items-center justify-between">
          <BrandWordmark />
          <span className="text-sm text-muted-foreground">Dashboard</span>
        </header>
        <section className="flex flex-1 items-center justify-center py-10">
          {children}
        </section>
      </div>
    </main>
  );
}
