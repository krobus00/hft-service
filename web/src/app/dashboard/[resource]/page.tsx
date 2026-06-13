import { DashboardApp } from "@/components/organisms/dashboard-app";
import { resources } from "@/lib/resources";
import type { ResourceKey } from "@/types/api";

type DashboardResourcePageProps = {
  params: Promise<{
    resource: string;
  }>;
};

export default async function DashboardResourcePage({ params }: DashboardResourcePageProps) {
  const { resource } = await params;
  const key = normalizeResourceKey(resource);
  return <DashboardApp initialResourceKey={key} />;
}

function normalizeResourceKey(value: string): ResourceKey {
  const match = resources.find((resource) => resource.key === value);
  return match?.key ?? "orders";
}
