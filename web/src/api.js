// Thin client for the Go API. Session is an HttpOnly cookie; the CSRF token
// returned at sign-in is sent on every state-changing request.

let csrfToken = null;
let onUnauthorized = () => {};

export function setCsrfToken(token) {
  csrfToken = token;
}

export function setUnauthorizedHandler(fn) {
  onUnauthorized = fn;
}

export class ApiError extends Error {
  constructor(status, err) {
    super(err?.message || `Request failed (${status})`);
    this.status = status;
    this.code = err?.code;
    this.field = err?.field;
  }
}

async function request(method, path, body) {
  const headers = {};
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (method !== 'GET' && csrfToken) headers['X-CSRF-Token'] = csrfToken;
  let res;
  try {
    res = await fetch(path, {
      method,
      headers,
      credentials: 'same-origin',
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, { code: 'network', message: 'Cannot reach the booking server. Check the office network.' });
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (res.status === 401 && path !== '/api/login' && path !== '/api/session') onUnauthorized();
    throw new ApiError(res.status, data.error);
  }
  return data;
}

const enc = encodeURIComponent;

export const api = {
  session: () => request('GET', '/api/session'),
  login: (username, password) => request('POST', '/api/login', { username, password }),
  logout: () => request('POST', '/api/logout'),
  config: () => request('GET', '/api/config'),
  desks: () => request('GET', '/api/desks'),
  colleagues: () => request('GET', '/api/colleagues'),
  bookings: (from, to) => request('GET', `/api/bookings?from=${enc(from)}&to=${enc(to)}`),
  myBookings: () => request('GET', '/api/bookings/mine'),
  createBooking: (b) => request('POST', '/api/bookings', b),
  changeBooking: (id, b) => request('PATCH', `/api/bookings/${enc(id)}`, b),
  cancelBooking: (id) => request('POST', `/api/bookings/${enc(id)}/cancel`),
  admin: {
    employees: () => request('GET', '/api/admin/employees'),
    createEmployee: (e) => request('POST', '/api/admin/employees', e),
    updateEmployee: (id, e) => request('PATCH', `/api/admin/employees/${enc(id)}`, e),
    createDesk: (d) => request('POST', '/api/admin/desks', d),
    updateDesk: (id, d) => request('PATCH', `/api/admin/desks/${enc(id)}`, d),
    deleteDesk: (id) => request('DELETE', `/api/admin/desks/${enc(id)}`),
    updateSettings: (s) => request('PUT', '/api/admin/settings', s),
    backups: () => request('GET', '/api/admin/backups'),
    createBackup: () => request('POST', '/api/admin/backups'),
  },
};
