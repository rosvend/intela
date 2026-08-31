const tokenKey = "intela.token";

export function token() {
  return localStorage.getItem(tokenKey) || "";
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
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  const ct = res.headers.get("content-type") || "";
  if (ct.includes("json")) return res.json();
  return res;
}

// Lectura sin sesion. El listado ONI es publico (R-18): adjuntar el token
// y redirigir a /login ante un 401 convertiria la publicacion legal en una
// pagina interna.
export async function apiPublica(path: string) {
  const res = await fetch(path);
  if (res.status === 404) return null;
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  const ct = res.headers.get("content-type") || "";
  if (ct.includes("json")) return res.json();
  return res;
}

export function setToken(t: string) {
  localStorage.setItem(tokenKey, t);
}
