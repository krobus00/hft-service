import { cn } from "@/lib/utils";

type BrandWordmarkProps = {
  className?: string;
};

export function BrandWordmark({ className }: BrandWordmarkProps) {
  return (
    <span
      className={cn(
        "text-lg font-semibold tracking-normal text-foreground",
        className,
      )}
    >
      Krobot
    </span>
  );
}
