// api.js: the only module that calls fetch(). Every section module goes
// through here so CSRF header injection, error normalization, and
// same-origin credentials stay in exactly one place.

let csrfToken = '';
let csrfHeaderName = 'X-Gamelib-Csrf';

export function configureCSRF(token, headerName) {
  csrfToken = token || '';
  if (headerName) csrfHeaderName = headerName;
}

class ApiError extends Error {
  constructor(message, code, status, data) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
    this.data = data;
  }
}

async function request(method, path, body) {
  const headers = {};
  let payload;
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    payload = JSON.stringify(body);
  }
  if (method !== 'GET') {
    headers[csrfHeaderName] = csrfToken;
  }
  let response;
  try {
    response = await fetch(path, {
      method,
      headers,
      body: payload,
      credentials: 'same-origin',
    });
  } catch (networkErr) {
    throw new ApiError(
      'could not reach the dashboard server (is gamelib serve still running?)',
      'network_error',
      0,
      null,
    );
  }
  const text = await response.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch (parseErr) {
      data = null;
    }
  }
  if (!response.ok) {
    const message =
      (data && data.error && data.error.message) || `request failed with status ${response.status}`;
    const code = (data && data.error && data.error.code) || 'unknown_error';
    throw new ApiError(message, code, response.status, data);
  }
  return data;
}

export const api = {
  get(path) {
    return request('GET', path);
  },
  put(path, body) {
    return request('PUT', path, body);
  },
  post(path, body) {
    return request('POST', path, body === undefined ? {} : body);
  },
};

export { ApiError };
