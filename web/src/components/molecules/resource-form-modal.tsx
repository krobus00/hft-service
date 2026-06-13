import { Eye, EyeOff, Plus, Save, Trash2 } from "lucide-react";
import { FormEvent, useMemo, useState } from "react";

import { ModalShell } from "@/components/molecules/modal-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import type { ResourceConfig } from "@/types/api";

type FieldValue = string | number | boolean | string[] | Record<string, string>;

type ResourceFormModalProps = {
  title: string;
  resource: ResourceConfig;
  initialValue: Record<string, unknown>;
  enums: Record<string, string[]>;
  submitLabel: string;
  disabled?: boolean;
  onSubmit: (value: Record<string, unknown>) => Promise<void>;
  onClose: () => void;
};

export function ResourceFormModal({
  title,
  resource,
  initialValue,
  enums,
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
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setIsSubmitting(true);
    try {
      await onSubmit(normalizeSubmitValues(resource, values));
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
                  multiple: Boolean(resource.multiEnumFields?.includes(field)),
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
  multiple: boolean;
  disabled?: boolean;
  onChange: (value: FieldValue) => void;
}) {
  const { field, sampleValue, value, options, multiple, disabled, onChange } = props;
  if (options.length > 0) {
    if (multiple) {
      return (
        <MultiEnumField
          value={normalizeStringArray(value)}
          options={options}
          disabled={disabled}
          onChange={onChange}
        />
      );
    }

    return (
      <Select
        id={field}
        value={String(value ?? "")}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map((option) => (
          <option key={`${field}-${option}`} value={option}>
            {option || "None"}
          </option>
        ))}
      </Select>
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

  if (isTimestampField(field)) {
    return (
      <Input
        id={field}
        type="datetime-local"
        value={String(value ?? "")}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
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
        <div key={index} className="grid grid-cols-[1fr_1fr_auto] gap-2">
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

function MultiEnumField({
  value,
  options,
  disabled,
  onChange,
}: {
  value: string[];
  options: string[];
  disabled?: boolean;
  onChange: (value: FieldValue) => void;
}) {
  function toggle(option: string, checked: boolean) {
    if (checked) {
      onChange(Array.from(new Set([...value, option])));
      return;
    }
    onChange(value.filter((item) => item !== option));
  }

  return (
    <div className="max-h-52 overflow-auto rounded-md border bg-background p-2">
      <div className="grid gap-1">
        {options.map((option) => (
          <label
            key={option}
            className="flex min-h-9 items-center gap-2 rounded-md px-2 text-sm hover:bg-muted"
          >
            <input
              type="checkbox"
              checked={value.includes(option)}
              disabled={disabled}
              onChange={(event) => toggle(option, event.target.checked)}
            />
            <span className="break-all">{option}</span>
          </label>
        ))}
      </div>
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
    if (Array.isArray(rawValue)) {
      result[field] = normalizeStringArray(rawValue);
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
    if (isTimestampField(field)) {
      result[field] = toLocalDateTimeInput(rawValue);
      return result;
    }
    result[field] = stripOuterQuotes(String(rawValue ?? ""));
    return result;
  }, {});
}

function normalizeStringArray(value: unknown) {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }
  if (typeof value === "string") {
    return value
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return [];
}

function normalizeSubmitValues(
  resource: ResourceConfig,
  values: Record<string, FieldValue>,
) {
  return Object.entries(values).reduce<Record<string, unknown>>((result, [field, value]) => {
    const sampleValue = resource.sampleBody?.[field];
    if (field === "key" && typeof value === "string") {
      result[field] = stripOuterQuotes(value);
      return result;
    }
    if (isTimestampField(field) && typeof value === "string") {
      result[field] = fromLocalDateTimeInput(value);
      return result;
    }
    if (resource.key === "settings" && field === "value" && typeof value === "string") {
      result[field] = parseSettingValue(value);
      return result;
    }
    if (isPlainObject(sampleValue)) {
      result[field] = isPlainObject(value) ? value : normalizeObjectField(value);
      return result;
    }
    result[field] = value;
    return result;
  }, {});
}

function isTimestampField(field: string) {
  return field.endsWith("_at") || field.endsWith("_time");
}

function toLocalDateTimeInput(value: unknown) {
  if (!value) {
    return "";
  }
  const date = new Date(String(value));
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
  return local.toISOString().slice(0, 16);
}

function fromLocalDateTimeInput(value: string) {
  if (!value.trim()) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toISOString();
}

function parseSettingValue(value: string) {
  const raw = stripOuterQuotes(value).trim();
  if (raw === "true") {
    return true;
  }
  if (raw === "false") {
    return false;
  }
  if (/^-?\d+(\.\d+)?$/.test(raw)) {
    return Number(raw);
  }
  return raw;
}

function stripOuterQuotes(value: string) {
  const raw = value.trim();
  if (
    (raw.startsWith('"') && raw.endsWith('"')) ||
    (raw.startsWith("'") && raw.endsWith("'")) ||
    (raw.startsWith("`") && raw.endsWith("`"))
  ) {
    return raw.slice(1, -1);
  }
  return raw;
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
