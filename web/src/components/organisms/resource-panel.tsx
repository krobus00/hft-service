"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { ResourceDetailModal } from "@/components/molecules/resource-detail-modal";
import { ResourceFormModal } from "@/components/molecules/resource-form-modal";
import { ResourceTable } from "@/components/molecules/resource-table";
import { ResourceToolbar } from "@/components/molecules/resource-toolbar";
import {
  createResource,
  deleteResource,
  getResourceDetail,
  listResource,
  updateResource,
} from "@/lib/api-client";
import { canWriteResource } from "@/lib/rbac";
import type { ListQuery, PaginationMeta, ResourceConfig } from "@/types/api";
import type { AuthUser } from "@/types/auth";

type EditorState =
  | { mode: "create"; value: Record<string, unknown> }
  | { mode: "update"; id: string; value: Record<string, unknown> }
  | null;

type ResourcePanelProps = {
  resource: ResourceConfig;
  user: AuthUser;
};

export function ResourcePanel({ resource, user }: ResourcePanelProps) {
  const [query, setQuery] = useState<ListQuery>(() => ({
    page: 1,
    limit: 10,
    keyword: "",
    sortField: resource.defaultSort.field,
    sortDirection: resource.defaultSort.direction,
  }));
  const [items, setItems] = useState<Array<Record<string, unknown>>>([]);
  const [meta, setMeta] = useState<PaginationMeta | null>(null);
  const [detail, setDetail] = useState<Record<string, unknown> | null>(null);
  const [editor, setEditor] = useState<EditorState>(null);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(false);

  const canWrite = useMemo(() => canWriteResource(user, resource), [resource, user]);

  useEffect(() => {
    setQuery({
      page: 1,
      limit: 10,
      keyword: "",
      sortField: resource.defaultSort.field,
      sortDirection: resource.defaultSort.direction,
    });
    setItems([]);
    setMeta(null);
    setDetail(null);
    setEditor(null);
    setError("");
  }, [resource]);

  const loadItems = useCallback(async () => {
    setIsLoading(true);
    setError("");
    try {
      const result = await listResource(resource, query);
      setItems(result.items);
      setMeta(result.meta);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load data.");
    } finally {
      setIsLoading(false);
    }
  }, [query, resource]);

  useEffect(() => {
    void loadItems();
  }, [loadItems]);

  async function openDetail(id: string) {
    if (!id) {
      return;
    }
    setError("");
    try {
      const result = await getResourceDetail(resource, id);
      setDetail(result);
      setEditor(null);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load detail.");
    }
  }

  async function handleDelete(id: string) {
    if (!id || !canWrite) {
      return;
    }
    const confirmed = window.confirm(`Delete ${resource.label} ${id}?`);
    if (!confirmed) {
      return;
    }
    setError("");
    try {
      await deleteResource(resource, id);
      setDetail(null);
      await loadItems();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to delete row.");
    }
  }

  async function submitEditor(value: Record<string, unknown>) {
    if (!editor || !canWrite) {
      return;
    }
    if (editor.mode === "create") {
      const created = await createResource(resource, value);
      setDetail(created);
    } else {
      const updated = await updateResource(resource, editor.id, value);
      setDetail(updated);
    }
    setEditor(null);
    await loadItems();
  }

  function openCreate() {
    setDetail(null);
    setEditor({
      mode: "create",
      value: resource.sampleBody ?? {},
    });
  }

  function openUpdate() {
    if (!detail) {
      return;
    }
    const id = String(detail[resource.idField] ?? "");
    if (!id) {
      return;
    }
    setEditor({ mode: "update", id, value: detail });
  }

  return (
    <section className="grid gap-5">
      <ResourceToolbar
        keyword={query.keyword}
        canWrite={canWrite}
        isLoading={isLoading}
        onRefresh={loadItems}
        onCreate={openCreate}
        onKeywordChange={(keyword) =>
          setQuery((current) => ({
            ...current,
            page: 1,
            keyword,
          }))
        }
      />

      {error ? (
        <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </p>
      ) : null}

      <ResourceTable
        resource={resource}
        items={items}
        meta={meta}
        canWrite={canWrite}
        onDetail={openDetail}
        onDelete={handleDelete}
        onPageChange={(page) => setQuery((current) => ({ ...current, page }))}
      />

      {editor ? (
        <ResourceFormModal
          title={editor.mode === "create" ? `Create ${resource.label}` : `Update ${resource.label}`}
          resource={resource}
          initialValue={editor.value}
          submitLabel={editor.mode === "create" ? "Create" : "Update"}
          disabled={!canWrite}
          onSubmit={submitEditor}
          onClose={() => setEditor(null)}
        />
      ) : null}

      {detail && !editor ? (
        <ResourceDetailModal
          title={`${resource.label} detail`}
          data={detail}
          canWrite={canWrite}
          onEdit={openUpdate}
          onClose={() => setDetail(null)}
        />
      ) : null}
    </section>
  );
}
