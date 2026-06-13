import { Eye, EyeOff } from "lucide-react";
import { useState } from "react";

import { FieldError } from "@/components/atoms/field-error";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type AuthFormFieldProps = {
  id: string;
  label: string;
  type?: string;
  value: string;
  autoComplete?: string;
  error?: string;
  onChange: (value: string) => void;
};

export function AuthFormField({
  id,
  label,
  type = "text",
  value,
  autoComplete,
  error,
  onChange,
}: AuthFormFieldProps) {
  const [showPassword, setShowPassword] = useState(false);
  const isPassword = type === "password";

  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      <div className="relative">
        <Input
          id={id}
          type={isPassword && showPassword ? "text" : type}
          value={value}
          autoComplete={autoComplete}
          aria-invalid={Boolean(error)}
          className={isPassword ? "pr-10" : undefined}
          onChange={(event) => onChange(event.target.value)}
        />
        {isPassword ? (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="absolute right-0 top-0 h-10 w-10"
            onClick={() => setShowPassword((current) => !current)}
          >
            {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
          </Button>
        ) : null}
      </div>
      <FieldError message={error} />
    </div>
  );
}
