import { Edit } from "lucide-react";

import { ModalShell } from "@/components/molecules/modal-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type ResourceDetailModalProps = {
  title: string;
  data: Record<string, unknown>;
  canWrite: boolean;
  onEdit: () => void;
  onClose: () => void;
};

export function ResourceDetailModal({
  title,
  data,
  canWrite,
  onEdit,
  onClose,
}: ResourceDetailModalProps) {
  return (
    <ModalShell title={title} onClose={onClose}>
      <div className="grid gap-4">
        <div className="grid gap-4 md:grid-cols-2">
          {Object.entries(data).map(([key, value]) => (
            <div key={key} className="grid gap-2">
              <Label htmlFor={`detail-${key}`}>{humanize(key)}</Label>
              <Input id={`detail-${key}`} value={formatValue(value)} readOnly />
            </div>
          ))}
        </div>
        <div className="flex justify-end">
          <Button type="button" onClick={onEdit} disabled={!canWrite}>
            <Edit className="h-4 w-4" />
            Edit
          </Button>
        </div>
      </div>
    </ModalShell>
  );
}

function humanize(value: string) {
  return value
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function formatValue(value: unknown): string {
  if (value == null) {
    return "";
  }
  if (typeof value === "boolean") {
    return value ? "true" : "false";
  }
  if (typeof value === "object") {
    return Object.entries(value as Record<string, unknown>)
      .map(([key, nestedValue]) => `${key}: ${formatValue(nestedValue)}`)
      .join(", ");
  }
  return String(value);
}
