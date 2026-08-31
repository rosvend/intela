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
    const metodo = (init.method || "GET").toUpperCase();
    const esPostLogin = path.includes("/auth/session") && metodo === "POST";
    if (!esPostLogin && !path.includes("/sesiones")) {
      window.location.href = "/login";
    }
  }
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
