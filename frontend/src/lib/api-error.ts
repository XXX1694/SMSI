import { describeErrorCode } from './errors';
import { stringRecord } from './normalize';

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string | null;
  /** Per-field messages of a validation error (`error.fields`), keyed by field name. */
  readonly fields: Record<string, string>;

  constructor(status: number, code: string, message: string, requestId: string | null = null, fields: Record<string, string> = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.requestId = requestId;
    this.fields = fields;
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
        stringRecord(r.fields),
      );
    }
  }
  const fallback: Record<number, [string, string]> = {
    401: ['UNAUTHENTICATED', describeErrorCode('UNAUTHENTICATED')],
    403: ['FORBIDDEN', describeErrorCode('FORBIDDEN')],
    404: ['NOT_FOUND', describeErrorCode('NOT_FOUND')],
    429: ['RATE_LIMITED', describeErrorCode('RATE_LIMITED')],
  };
  const [code, message] = fallback[status] ?? [status >= 500 ? 'INTERNAL' : 'UNKNOWN', `Request failed (${status}).`];
  return new ApiError(status, code, message);
}
