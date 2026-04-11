"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";

import { ConfirmationModal } from "@/components/dashboard/confirmation-modal";
import { Modal } from "@/components/dashboard/modal";
import type {
  CreateUserRequest,
  DashboardUser,
  ListUsersPayload,
  UpdateUserRequest,
} from "@/lib/types/dashboard-user";

type ApiResponse<T> = {
  data?: T;
  error?: {
    code?: string;
    message: string;
  };
};

type UserFormState = {
  username: string;
  password: string;
  role: "admin" | "user";
  is_active: boolean;
};

const defaultForm: UserFormState = {
  username: "",
  password: "",
  role: "user",
  is_active: true,
};

const pageSize = 10;

export function UsersManager() {
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);

  const [loadingList, setLoadingList] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  const [items, setItems] = useState<DashboardUser[]>([]);
  const [pagination, setPagination] = useState<ListUsersPayload["pagination"]>({
    page: 1,
    page_size: pageSize,
    total_items: 0,
    total_pages: 1,
    has_next: false,
    has_prev: false,
  });

  const [selectedUser, setSelectedUser] = useState<DashboardUser | null>(null);
  const [editing, setEditing] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);
  const [pendingDeleteUser, setPendingDeleteUser] = useState<DashboardUser | null>(null);
  const [form, setForm] = useState<UserFormState>(defaultForm);
  const [error, setError] = useState("");

  const mode = useMemo(() => {
    if (editing && selectedUser) {
      return "edit" as const;
    }

    return "create" as const;
  }, [editing, selectedUser]);

  const loadUsers = async () => {
    setLoadingList(true);

    try {
      const query = new URLSearchParams({
        page: String(page),
        page_size: String(pageSize),
      });

      if (search.trim()) {
        query.set("search", search.trim());
      }

      const response = await fetch(`/api/dashboard/users?${query.toString()}`, {
        method: "GET",
        cache: "no-store",
        credentials: "include",
      });

      const payload = (await response.json()) as ApiResponse<ListUsersPayload>;

      if (!response.ok || !payload.data) {
        setError(payload.error?.message ?? "Failed to fetch users");
        return;
      }

      setItems(payload.data.items);
      setPagination(payload.data.pagination);

      if (selectedUser) {
        const updated = payload.data.items.find((user) => user.id === selectedUser.id);
        if (!updated) {
          setSelectedUser(null);
          setEditing(false);
          setForm(defaultForm);
        }
      }
    } catch {
      setError("Failed to fetch users");
    } finally {
      setLoadingList(false);
    }
  };

  useEffect(() => {
    void loadUsers();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, search]);

  const loadUserDetail = async (userId: string) => {
    setError("");

    try {
      const response = await fetch(`/api/dashboard/users/${userId}`, {
        method: "GET",
        cache: "no-store",
        credentials: "include",
      });
      const payload = (await response.json()) as ApiResponse<DashboardUser>;

      if (!response.ok || !payload.data) {
        setError(payload.error?.message ?? "Failed to fetch user detail");
        return;
      }

      setSelectedUser(payload.data);
      setEditing(true);
      setModalOpen(true);
      setForm({
        username: payload.data.username,
        password: "",
        role: payload.data.role,
        is_active: payload.data.is_active,
      });
    } catch {
      setError("Failed to fetch user detail");
    }
  };

  const onSearchSubmit = (event: FormEvent) => {
    event.preventDefault();
    setPage(1);
    setSearch(searchInput.trim());
  };

  const onResetForm = () => {
    setEditing(false);
    setSelectedUser(null);
    setForm(defaultForm);
    setModalOpen(false);
    setError("");
  };

  const onOpenCreateModal = () => {
    setEditing(false);
    setSelectedUser(null);
    setForm(defaultForm);
    setError("");
    setModalOpen(true);
  };

  const onSubmitUser = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setError("");

    try {
      if (mode === "create") {
        const payload: CreateUserRequest = {
          username: form.username.trim(),
          password: form.password,
          role: form.role,
          is_active: form.is_active,
        };

        const response = await fetch("/api/dashboard/users", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          credentials: "include",
          body: JSON.stringify(payload),
        });

        const result = (await response.json()) as ApiResponse<DashboardUser>;
        if (!response.ok || !result.data) {
          setError(result.error?.message ?? "Failed to create user");
          return;
        }

        setSelectedUser(result.data);
        setEditing(true);
        setModalOpen(true);
        setForm({
          username: result.data.username,
          password: "",
          role: result.data.role,
          is_active: result.data.is_active,
        });
      } else if (selectedUser) {
        const payload: UpdateUserRequest = {
          username: form.username.trim(),
          role: form.role,
          is_active: form.is_active,
        };

        if (form.password.trim()) {
          payload.password = form.password;
        }

        const response = await fetch(`/api/dashboard/users/${selectedUser.id}`, {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          credentials: "include",
          body: JSON.stringify(payload),
        });

        const result = (await response.json()) as ApiResponse<DashboardUser>;
        if (!response.ok || !result.data) {
          setError(result.error?.message ?? "Failed to update user");
          return;
        }

        setSelectedUser(result.data);
        setModalOpen(true);
        setForm({
          username: result.data.username,
          password: "",
          role: result.data.role,
          is_active: result.data.is_active,
        });
      }

      await loadUsers();
    } catch {
      setError(mode === "create" ? "Failed to create user" : "Failed to update user");
    } finally {
      setSubmitting(false);
    }
  };

  const onDeleteUser = async (userId: string) => {
    setError("");

    try {
      const response = await fetch(`/api/dashboard/users/${userId}`, {
        method: "DELETE",
        credentials: "include",
      });

      const result = (await response.json()) as ApiResponse<{ id: string }>;
      if (!response.ok) {
        setError(result.error?.message ?? "Failed to delete user");
        return;
      }

      if (selectedUser?.id === userId) {
        onResetForm();
      }

      setConfirmDeleteOpen(false);
      setPendingDeleteUser(null);

      await loadUsers();
    } catch {
      setError("Failed to delete user");
    }
  };

  const onAskDeleteUser = (user: DashboardUser) => {
    setPendingDeleteUser(user);
    setConfirmDeleteOpen(true);
  };

  return (
    <section className="users-section stack-gap">
      <div className="panel users-toolbar">
        <div>
          <p className="eyebrow">User Management</p>
          <h2>Manage Dashboard Users</h2>
        </div>

        <form onSubmit={onSearchSubmit} className="users-search-form">
          <input
            type="search"
            placeholder="Search username"
            value={searchInput}
            onChange={(event) => setSearchInput(event.target.value)}
          />
          <button type="submit" className="primary-button">Search</button>
        </form>
      </div>

      {error ? <p className="auth-error">{error}</p> : null}

      <div className="dashboard-grid users-grid">
        <article className="panel users-list-panel">
          <div className="users-list-header">
            <h2>Users</h2>
            <button type="button" className="logout-button" onClick={onOpenCreateModal}>
              New User
            </button>
          </div>

          {loadingList ? (
            <p className="stat-meta">Loading users...</p>
          ) : (
            <div className="users-table-wrap">
              <table className="users-table">
                <thead>
                  <tr>
                    <th>Username</th>
                    <th>Role</th>
                    <th>Status</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((user) => (
                    <tr key={user.id}>
                      <td>{user.username}</td>
                      <td>{user.role}</td>
                      <td>{user.is_active ? "Active" : "Inactive"}</td>
                      <td className="users-actions">
                        <button type="button" onClick={() => void loadUserDetail(user.id)}>
                          Detail
                        </button>
                        <button type="button" onClick={() => onAskDeleteUser(user)}>
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div className="users-pagination">
            <button
              type="button"
              className="logout-button"
              disabled={!pagination.has_prev}
              onClick={() => setPage((prev) => Math.max(1, prev - 1))}
            >
              Prev
            </button>
            <p className="stat-meta">
              Page {pagination.page} / {pagination.total_pages} ({pagination.total_items} users)
            </p>
            <button
              type="button"
              className="logout-button"
              disabled={!pagination.has_next}
              onClick={() => setPage((prev) => prev + 1)}
            >
              Next
            </button>
          </div>
        </article>
      </div>

      <Modal
        open={modalOpen}
        onClose={onResetForm}
        title={mode === "create" ? "Create User" : "User Detail & Update"}
        subtitle={mode === "create" ? "Add a new dashboard user" : `User ID: ${selectedUser?.id ?? "-"}`}
        footer={
          <>
            <button type="button" className="modal-button secondary" onClick={onResetForm}>
              Close
            </button>
            <button
              type="submit"
              form="user-modal-form"
              className="modal-button primary"
              disabled={submitting}
            >
              {submitting
                ? "Saving..."
                : mode === "create"
                  ? "Create User"
                  : "Update User"}
            </button>
          </>
        }
      >
        <form id="user-modal-form" onSubmit={onSubmitUser} className="users-form">
          <label htmlFor="user-username">Username</label>
          <input
            id="user-username"
            value={form.username}
            onChange={(event) => setForm((prev) => ({ ...prev, username: event.target.value }))}
            required
          />

          <label htmlFor="user-password">
            Password {mode === "edit" ? "(optional for update)" : ""}
          </label>
          <input
            id="user-password"
            type="password"
            value={form.password}
            onChange={(event) => setForm((prev) => ({ ...prev, password: event.target.value }))}
            required={mode === "create"}
          />

          <label htmlFor="user-role">Role</label>
          <select
            id="user-role"
            value={form.role}
            onChange={(event) =>
              setForm((prev) => ({
                ...prev,
                role: event.target.value as "admin" | "user",
              }))
            }
          >
            <option value="user">User</option>
            <option value="admin">Admin</option>
          </select>

          <label className="users-checkbox">
            <input
              type="checkbox"
              checked={form.is_active}
              onChange={(event) =>
                setForm((prev) => ({ ...prev, is_active: event.target.checked }))
              }
            />
            Active account
          </label>
        </form>
      </Modal>

      <ConfirmationModal
        open={confirmDeleteOpen}
        title="Delete user"
        description={`Are you sure you want to delete ${pendingDeleteUser?.username ?? "this user"}?`}
        confirmLabel="Delete"
        loading={submitting}
        onCancel={() => {
          setConfirmDeleteOpen(false);
          setPendingDeleteUser(null);
        }}
        onConfirm={() => {
          if (pendingDeleteUser) {
            void onDeleteUser(pendingDeleteUser.id);
          }
        }}
      />
    </section>
  );
}
