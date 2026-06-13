import type { Permission, ResourceConfig } from "@/types/api";
import type { AuthUser } from "@/types/auth";

export function isAdmin(user: AuthUser | null) {
  return Boolean(user?.roles.includes("admin"));
}

export function hasPermission(user: AuthUser | null, permission?: Permission) {
  if (!user || !permission) {
    return false;
  }
  if (isAdmin(user)) {
    return true;
  }
  return user.permissions.includes(permission);
}

export function canReadResource(user: AuthUser | null, resource: ResourceConfig) {
  return hasPermission(user, resource.readPermission);
}

export function canWriteResource(user: AuthUser | null, resource: ResourceConfig) {
  return resource.mode === "mutable" && hasPermission(user, resource.writePermission);
}
