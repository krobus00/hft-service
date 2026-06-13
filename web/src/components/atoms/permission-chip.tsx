import { Badge } from "@/components/ui/badge";
import type { Permission } from "@/types/api";

type PermissionChipProps = {
  permission: Permission;
  allowed: boolean;
};

export function PermissionChip({ permission, allowed }: PermissionChipProps) {
  return (
    <Badge variant={allowed ? "secondary" : "outline"} className="whitespace-nowrap">
      {permission}
    </Badge>
  );
}
