import { X } from "lucide-react";
import { ReactNode } from "react";

import { Button } from "@/components/ui/button";

type ModalShellProps = {
  title: string;
  children: ReactNode;
  size?: "md" | "wide";
  onClose: () => void;
};

export function ModalShell({ title, children, size = "md", onClose }: ModalShellProps) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/30 p-4">
      <section className={`max-h-[90vh] w-full overflow-hidden rounded-md border bg-card shadow-lg ${size === "wide" ? "max-w-6xl" : "max-w-3xl"}`}>
        <header className="flex items-center justify-between border-b px-4 py-3">
          <h3 className="text-sm font-semibold">{title}</h3>
          <Button type="button" variant="ghost" size="icon" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </header>
        <div className="max-h-[calc(90vh-3.5rem)] overflow-auto p-4">{children}</div>
      </section>
    </div>
  );
}
