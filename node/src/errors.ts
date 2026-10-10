/** What the API answers when it refuses a request. Branch on `code`; the rest is prose. */
export class GagarinError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fixHint?: string;

  constructor(status: number, code: string, message: string, fixHint?: string) {
    super(fixHint ? `[${code}] ${message}\nhint: ${fixHint}` : `[${code}] ${message}`);
    this.name = "GagarinError";
    this.status = status;
    this.code = code;
    this.fixHint = fixHint;
  }
}

/** Builds the error for a refusal, from the API's envelope when there is one. */
export function apiError(status: number, text: string): GagarinError {
  let e: { code?: string; message?: string; fix_hint?: string } | undefined;
  try {
    e = JSON.parse(text)?.error;
  } catch {
    /* not the envelope */
  }
  if (e?.code) return new GagarinError(status, e.code, e.message ?? "", e.fix_hint);
  return new GagarinError(status, `http_${status}`, text.trim());
}
