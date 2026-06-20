import { cn } from "@/lib/utils";

type BrandWordmarkProps = {
  className?: string;
};

export function BrandWordmark({ className }: BrandWordmarkProps) {
  return (
    <span
      className={cn("inline-flex items-center gap-2 text-lg font-bold tracking-tight", className)}
    >
      <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-primary to-emerald-400 text-sm text-primary-foreground shadow-sm shadow-primary/25">
        K
      </span>
      <span>Krobot</span>
    </span>
  );
}
