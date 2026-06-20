"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { ResourceDetailModal } from "@/components/molecules/resource-detail-modal";
import { ResourceFormModal } from "@/components/molecules/resource-form-modal";
import { ResourceTable } from "@/components/molecules/resource-table";
import { ResourceToolbar } from "@/components/molecules/resource-toolbar";
import {
  createResource,
  deleteResource,
  getResourceDetail,
  getFormEnums,
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
    filters: {},
    sortField: resource.defaultSort.field,
    sortDirection: resource.defaultSort.direction,
  }));
  const [debouncedQuery, setDebouncedQuery] = useState<ListQuery>(query);
  const [items, setItems] = useState<Array<Record<string, unknown>>>([]);
  const [meta, setMeta] = useState<PaginationMeta | null>(null);
  const [detail, setDetail] = useState<Record<string, unknown> | null>(null);
  const [editor, setEditor] = useState<EditorState>(null);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [pendingAction, setPendingAction] = useState("");
  const [enums, setEnums] = useState<Record<string, string[]>>({});
  const loadSeq = useRef(0);

  const canWrite = useMemo(() => canWriteResource(user, resource), [resource, user]);

  useEffect(() => {
    const timeout = globalThis.setTimeout(() => {
      setDebouncedQuery(query);
    }, 300);
    return () => globalThis.clearTimeout(timeout);
  }, [query]);

  useEffect(() => {
    let isMounted = true;
    const needsEnums = Boolean(
      resource.enumFields &&
        (Object.keys(resource.enumFields).length > 0 || (resource.filterFields?.length ?? 0) > 0),
    );
    if (!needsEnums) {
      setEnums({});
      return;
    }
    async function loadEnums() {
      try {
        const result = await getFormEnums();
        if (isMounted) {
          setEnums(result);
        }
      } catch {
        if (isMounted) {
          setEnums({});
        }
      }
    }
    void loadEnums();
    return () => {
      isMounted = false;
    };
  }, [resource]);

  const loadItems = useCallback(async (nextQuery = debouncedQuery) => {
    const seq = loadSeq.current + 1;
    loadSeq.current = seq;
    setIsLoading(true);
    setError("");
    try {
      const result = await listResource(resource, nextQuery);
      if (loadSeq.current === seq) {
        setItems(result.items);
        setMeta(result.meta);
      }
    } catch (caught) {
      if (loadSeq.current === seq) {
        setError(caught instanceof Error ? caught.message : "Unable to load data.");
      }
    } finally {
      if (loadSeq.current === seq) {
        setIsLoading(false);
      }
    }
  }, [debouncedQuery, resource]);

  useEffect(() => {
    void loadItems();
  }, [loadItems]);

  async function openDetail(id: string) {
    if (!id || pendingAction) {
      return;
    }
    setPendingAction(`detail:${id}`);
    setError("");
    try {
      const result = await getResourceDetail(resource, id);
      setDetail(result);
      setEditor(null);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to load detail.");
    } finally {
      setPendingAction("");
    }
  }

  async function handleDelete(id: string) {
    if (!id || !canWrite || pendingAction) {
      return;
    }
    const confirmed = window.confirm(`Delete ${resource.label} ${id}?`);
    if (!confirmed) {
      return;
    }
    setPendingAction(`delete:${id}`);
    setError("");
    try {
      await deleteResource(resource, id);
      setDetail(null);
      await loadItems();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to delete row.");
    } finally {
      setPendingAction("");
    }
  }

  async function submitEditor(value: Record<string, unknown>) {
    if (!editor || !canWrite || pendingAction) {
      return;
    }
    setPendingAction("submit");
    try {
      if (editor.mode === "create") {
        const created = await createResource(resource, value);
        setDetail(created);
      } else {
        const updated = await updateResource(resource, editor.id, value);
        setDetail(updated);
      }
      setEditor(null);
      await loadItems();
    } finally {
      setPendingAction("");
    }
  }

  function openCreate() {
    if (pendingAction) {
      return;
    }
    setDetail(null);
    setEditor({
      mode: "create",
      value: resource.sampleBody ?? {},
    });
  }

  function openUpdate() {
    if (!detail || pendingAction) {
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
        filters={query.filters}
        filterFields={resource.filterFields ?? []}
        enumFields={resource.enumFields}
        enums={enums}
        canWrite={canWrite}
        isLoading={isLoading}
        onRefresh={() => loadItems(query)}
        onCreate={openCreate}
        onKeywordChange={(keyword) =>
          setQuery((current) => ({
            ...current,
            page: 1,
            keyword,
          }))
        }
        onFilterChange={(field, value) =>
          setQuery((current) => ({
            ...current,
            page: 1,
            filters: { ...current.filters, [field]: value },
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
        sortField={query.sortField}
        sortDirection={query.sortDirection}
        onDetail={openDetail}
        onDelete={handleDelete}
        onPageChange={(page) => setQuery((current) => ({ ...current, page }))}
        onSortChange={(field) =>
          setQuery((current) => ({
            ...current,
            page: 1,
            sortField: field,
            sortDirection:
              current.sortField === field && current.sortDirection === "asc" ? "desc" : "asc",
          }))
        }
      />

      {editor ? (
        <ResourceFormModal
          title={editor.mode === "create" ? `Create ${resource.label}` : `Update ${resource.label}`}
          resource={resource}
          initialValue={editor.value}
          enums={enums}
          submitLabel={editor.mode === "create" ? "Create" : "Update"}
          disabled={!canWrite || pendingAction === "submit"}
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
