import { Eye, EyeOff, Plus, Save, Trash2 } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";

import { ModalShell } from "@/components/molecules/modal-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { getFormEnums } from "@/lib/api-client";
import type { ResourceConfig } from "@/types/api";

type FieldValue = string | number | boolean | Record<string, string>;

type ResourceFormModalProps = {
  title: string;
  resource: ResourceConfig;
  initialValue: Record<string, unknown>;
  submitLabel: string;
  disabled?: boolean;
  onSubmit: (value: Record<string, unknown>) => Promise<void>;
  onClose: () => void;
};

export function ResourceFormModal({
  title,
  resource,
  initialValue,
  submitLabel,
  disabled,
  onSubmit,
  onClose,
}: ResourceFormModalProps) {
  const fields = useMemo(
    () => Object.keys(resource.sampleBody ?? initialValue).filter((field) => field !== resource.idField),
    [initialValue, resource],
  );
  const [values, setValues] = useState<Record<string, FieldValue>>(() =>
    buildInitialValues(fields, resource.sampleBody ?? {}, initialValue),
  );
  const [enums, setEnums] = useState<Record<string, string[]>>({});
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    let isMounted = true;
    async function loadEnums() {
      if (!resource.enumFields || Object.keys(resource.enumFields).length === 0) {
        return;
      }
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
  }, [resource.enumFields]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setIsSubmitting(true);
    try {
      await onSubmit(values);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Request failed.");
    } finally {
      setIsSubmitting(false);
    }
  }

  function updateField(field: string, value: FieldValue) {
    setValues((current) => ({ ...current, [field]: value }));
  }

  return (
    <ModalShell title={title} onClose={onClose}>
      <form className="grid gap-4" onSubmit={handleSubmit}>
        <div className="grid gap-4 md:grid-cols-2">
          {fields.map((field) => {
            const sampleValue = resource.sampleBody?.[field] ?? initialValue[field];
            const value = values[field];
            return (
              <div key={field} className="grid gap-2">
                <Label htmlFor={field}>{humanize(field)}</Label>
                {renderField({
                  field,
                  sampleValue,
                  value,
                  options: enumOptions(resource, enums, field),
                  disabled: disabled || isSubmitting,
                  onChange: (nextValue) => updateField(field, nextValue),
                })}
              </div>
            );
          })}
        </div>
        {error ? (
          <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </p>
        ) : null}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={disabled || isSubmitting}>
            <Save className="h-4 w-4" />
            {submitLabel}
          </Button>
        </div>
      </form>
    </ModalShell>
  );
}

function renderField(props: {
  field: string;
  sampleValue: unknown;
  value: FieldValue | undefined;
  options: string[];
  disabled?: boolean;
  onChange: (value: FieldValue) => void;
}) {
  const { field, sampleValue, value, options, disabled, onChange } = props;
  if (options.length > 0) {
    return (
      <select
        id={field}
        value={String(value ?? "")}
        disabled={disabled}
        className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map((option) => (
          <option key={`${field}-${option}`} value={option}>
            {option || "None"}
          </option>
        ))}
      </select>
    );
  }

  if (typeof sampleValue === "boolean") {
    return (
      <label className="flex h-10 items-center gap-2 rounded-md border px-3 text-sm">
        <input
          type="checkbox"
          checked={Boolean(value)}
          disabled={disabled}
          onChange={(event) => onChange(event.target.checked)}
        />
        Enabled
      </label>
    );
  }

  if (typeof sampleValue === "number") {
    return (
      <Input
        id={field}
        type="number"
        value={String(value ?? "")}
        disabled={disabled}
        onChange={(event) => onChange(Number(event.target.value))}
      />
    );
  }

  if (isPlainObject(sampleValue) || isPlainObject(value)) {
    return (
      <ObjectField
        value={normalizeObjectField(value)}
        disabled={disabled}
        onChange={onChange}
      />
    );
  }

  return (
    <PasswordAwareInput
      id={field}
      value={String(value ?? "")}
      disabled={disabled}
      onChange={(nextValue) => onChange(nextValue)}
    />
  );
}

function PasswordAwareInput({
  id,
  value,
  disabled,
  onChange,
}: {
  id: string;
  value: string;
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  const [showPassword, setShowPassword] = useState(false);
  const isPassword = id.toLowerCase().includes("password");

  if (!isPassword) {
    return (
      <Input
        id={id}
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
      />
    );
  }

  return (
    <div className="relative">
      <Input
        id={id}
        type={showPassword ? "text" : "password"}
        value={value}
        disabled={disabled}
        className="pr-10"
        onChange={(event) => onChange(event.target.value)}
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute right-0 top-0 h-10 w-10"
        disabled={disabled}
        onClick={() => setShowPassword((current) => !current)}
      >
        {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </Button>
    </div>
  );
}

function ObjectField({
  value,
  disabled,
  onChange,
}: {
  value: Record<string, string>;
  disabled?: boolean;
  onChange: (value: FieldValue) => void;
}) {
  const entries = Object.entries(value);

  function updateKey(index: number, nextKey: string) {
    const next = { ...value };
    const [oldKey, oldValue] = entries[index];
    delete next[oldKey];
    next[nextKey] = oldValue;
    onChange(next);
  }

  function updateValue(key: string, nextValue: string) {
    onChange({ ...value, [key]: nextValue });
  }

  function removeKey(key: string) {
    const next = { ...value };
    delete next[key];
    onChange(next);
  }

  function addKey() {
    let index = entries.length + 1;
    let key = `field_${index}`;
    while (key in value) {
      index += 1;
      key = `field_${index}`;
    }
    onChange({ ...value, [key]: "" });
  }

  return (
    <div className="grid gap-2 rounded-md border p-2 md:col-span-2">
      {entries.map(([key, nestedValue], index) => (
        <div key={`${key}-${index}`} className="grid grid-cols-[1fr_1fr_auto] gap-2">
          <Input
            value={key}
            disabled={disabled}
            aria-label="Object key"
            onChange={(event) => updateKey(index, event.target.value)}
          />
          <Input
            value={nestedValue}
            disabled={disabled}
            aria-label="Object value"
            onChange={(event) => updateValue(key, event.target.value)}
          />
          <Button
            type="button"
            variant="outline"
            size="icon"
            disabled={disabled}
            onClick={() => removeKey(key)}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      ))}
      <Button type="button" variant="outline" size="sm" disabled={disabled} onClick={addKey}>
        <Plus className="h-4 w-4" />
        Add field
      </Button>
    </div>
  );
}

function buildInitialValues(
  fields: string[],
  sample: Record<string, unknown>,
  initial: Record<string, unknown>,
) {
  return fields.reduce<Record<string, FieldValue>>((result, field) => {
    const sampleValue = sample[field];
    const rawValue = initial[field] ?? sampleValue ?? "";
    if (isPlainObject(sampleValue) || isPlainObject(rawValue)) {
      result[field] = normalizeObjectField(rawValue);
      return result;
    }
    if (typeof sampleValue === "boolean") {
      result[field] = Boolean(rawValue);
      return result;
    }
    if (typeof sampleValue === "number") {
      result[field] = Number(rawValue);
      return result;
    }
    result[field] = String(rawValue ?? "");
    return result;
  }, {});
}

function normalizeObjectField(value: unknown) {
  const parsed = typeof value === "string" ? parseObjectString(value) : value;
  if (!isPlainObject(parsed)) {
    return {};
  }
  return Object.entries(parsed).reduce<Record<string, string>>((result, [key, nestedValue]) => {
    result[key] = nestedValue == null ? "" : String(nestedValue);
    return result;
  }, {});
}

function parseObjectString(value: string) {
  try {
    return JSON.parse(value);
  } catch {
    return {};
  }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function enumOptions(
  resource: ResourceConfig,
  enums: Record<string, string[]>,
  field: string,
) {
  const enumKey = resource.enumFields?.[field];
  if (!enumKey) {
    return [];
  }
  return enums[enumKey] ?? [];
}

function humanize(value: string) {
  return value
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}
