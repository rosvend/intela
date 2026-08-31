const tokenKey = "intela.token";

export function token() {
  return localStorage.getItem(tokenKey) || "";
}

async function mensajeDeError(res: Response): Promise<string> {
  const t = await res.text();
  try {
    const j = JSON.parse(t) as { error?: string };
    if (typeof j.error === "string" && j.error) return j.error;
  } catch {
    // El cuerpo no era JSON; se usa el texto crudo.
  }
  return t || res.statusText;
}

export async function api(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers);
  if (token()) headers.set("Authorization", `Bearer ${token()}`);
  if (
    !(init.body instanceof FormData) &&
    !headers.has("Content-Type") &&
    init.body
  ) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(path, { ...init, headers });
  if (res.status === 401) {
    localStorage.removeItem(tokenKey);
    if (!path.includes("/sesiones")) window.location.href = "/login";
  }
  if (!res.ok) {
    throw new Error(await mensajeDeError(res));
  }
  const ct = res.headers.get("content-type") || "";
  if (ct.includes("json")) return res.json();
  return res;
}

export function setToken(t: string) {
  localStorage.setItem(tokenKey, t);
}
