export let csrf = "";
export function setCSRF(value: string) {
  csrf = value;
}
export class APIError extends Error {
  constructor(
    message: string,
    public code: string,
  ) {
    super(message);
  }
}
export async function api<T>(
  route: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch(route, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-Fragment-CSRF": csrf },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await response.json();
  if (!response.ok)
    throw new APIError(
      data.error
        ? `${data.message || data.error.message} (${data.error.code})`
        : `リクエストに失敗しました (${response.status})`,
      data.error?.code || "request_failed",
    );
  return data;
}
