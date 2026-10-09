export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string | null;

  constructor(status: number, code: string, message: string, requestId: string | null = null) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

/** Parse the uniform `{"error":{code,message,request_id}}` format; tolerate anything else. */
export function parseErrorBody(status: number, body: unknown): ApiError {
  if (typeof body === 'object' && body !== null && 'error' in body) {
    const e = (body as { error: unknown }).error;
    if (typeof e === 'object' && e !== null) {
      const r = e as Record<string, unknown>;
      return new ApiError(
        status,
        typeof r.code === 'string' ? r.code : 'UNKNOWN',
        typeof r.message === 'string' ? r.message : `Request failed (${status})`,
        typeof r.request_id === 'string' ? r.request_id : null,
      );
    }
  }
  const fallback: Record<number, [string, string]> = {
    401: ['UNAUTHENTICATED', 'Please sign in.'],
    403: ['FORBIDDEN', 'You are not allowed to do that.'],
    404: ['NOT_FOUND', 'Not found.'],
    429: ['RATE_LIMITED', 'Too many requests. Try again shortly.'],
  };
  const [code, message] = fallback[status] ?? [status >= 500 ? 'INTERNAL' : 'UNKNOWN', `Request failed (${status}).`];
  return new ApiError(status, code, message);
}
