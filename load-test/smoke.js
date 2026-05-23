import http from 'k6/http';
import { check } from 'k6';

const BASE_ORIGIN = __ENV.BASE_ORIGIN || 'http://localhost:8001';
const BASE_URL = __ENV.BASE_URL || `${BASE_ORIGIN}/v1`;
const EMPLOYEE_ID = __ENV.SMOKE_EMPLOYEE_ID || 'EMP001';
const PASSWORD = __ENV.SMOKE_PASSWORD || 'password';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1'],
    http_req_failed: ['rate==0'],
  },
};

export default function () {
  const health = http.get(`${BASE_ORIGIN}/health`, { timeout: '10s' });
  check(health, { 'health is 200': (r) => r.status === 200 });

  const ready = http.get(`${BASE_ORIGIN}/ready`, { timeout: '10s' });
  check(ready, { 'ready is 200': (r) => r.status === 200 });

  const login = http.post(`${BASE_URL}/auth/login`, JSON.stringify({
    employee_id: EMPLOYEE_ID,
    password: PASSWORD,
  }), {
    headers: { 'Content-Type': 'application/json' },
    timeout: '10s',
  });
  check(login, { 'login is 200': (r) => r.status === 200 });
}
