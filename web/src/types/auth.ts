export type ApiEnvelope<T> = {
  message: string;
  code: string;
  data?: T;
  validation_errors?: Array<{
    field: string;
    tag: string;
    message: string;
  }>;
};

export type AuthUser = {
  id: string;
  email: string;
  name: string;
  roles: string[];
  permissions: string[];
};

export type AuthTokens = {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
  refresh_token_expires_in: number;
};

export type AuthResult = {
  user: AuthUser;
  tokens: AuthTokens;
};

export type LoginPayload = {
  email: string;
  password: string;
};

export type SetupPayload = LoginPayload & {
  name: string;
};

export type Session = {
  user: AuthUser;
  tokens: AuthTokens;
};
