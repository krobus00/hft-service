import { cn } from "@/lib/utils";

type BrandWordmarkProps = {
  className?: string;
};

export function BrandWordmark({ className }: BrandWordmarkProps) {
  return (
    <span
      className={cn("inline-flex items-center gap-2 text-lg font-bold tracking-tight", className)}
    >
      <span>Krobot</span>
    </span>
  );
}
