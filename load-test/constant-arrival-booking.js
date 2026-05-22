import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import crypto from 'k6/crypto';
import encoding from 'k6/encoding';
import { SharedArray } from 'k6/data';
import { Counter, Rate } from 'k6/metrics';

const envData = new SharedArray('stress env', function () {
  return [JSON.parse(open('./stress_env.json'))];
})[0];

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8001/v1';
const RATE = Number(__ENV.RATE || 100);
const TIME_UNIT = __ENV.TIME_UNIT || '1s';
const DURATION = __ENV.DURATION || '1m';
const PRE_ALLOCATED_VUS = Number(__ENV.PRE_ALLOCATED_VUS || Math.max(100, Math.ceil(RATE / 2)));
const MAX_VUS = Number(__ENV.MAX_VUS || Math.max(PRE_ALLOCATED_VUS * 2, RATE));
const STRICT_THRESHOLDS = (__ENV.STRICT_THRESHOLDS || '0') === '1';
const DISCARD_RESPONSE_BODIES = (__ENV.DISCARD_RESPONSE_BODIES || '1') === '1';
const JWT_SECRET = __ENV.JWT_SECRET || 'dev-jwt-secret-change-in-prod-32chars!!';

export const options = {
  discardResponseBodies: DISCARD_RESPONSE_BODIES,
  scenarios: {
    booking_arrival_rate: {
      executor: 'constant-arrival-rate',
      rate: RATE,
      timeUnit: TIME_UNIT,
      duration: DURATION,
      preAllocatedVUs: PRE_ALLOCATED_VUS,
      maxVUs: MAX_VUS,
    },
  },
  thresholds: STRICT_THRESHOLDS
    ? {
        http_req_failed: ['rate<0.05'],
        http_req_duration: ['p(95)<3000'],
        booking_server_error: ['count==0'],
      }
    : {},
};

const bookingSuccess = new Counter('booking_success');
const bookingQueued202 = new Counter('booking_queued_202');
const bookingSoldOut409 = new Counter('booking_sold_out_409');
const bookingTooManyRequests429 = new Counter('booking_too_many_requests_429');
const bookingServerError = new Counter('booking_server_error');
const bookingOtherStatus = new Counter('booking_other_status');
const bookingSuccessRate = new Rate('booking_success_rate');
const bookingFailureRate = new Rate('booking_failure_rate');

function randString(length) {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
  let out = '';
  while (length--) out += chars[Math.random() * chars.length | 0];
  return out;
}

function forgeJWT(userIndex) {
  const realUserId = envData.users[userIndex % envData.users.length];
  const header = encoding.b64encode(JSON.stringify({ alg: 'HS256', typ: 'JWT' }), 'rawurl');
  const payload = encoding.b64encode(JSON.stringify({
    user_id: realUserId,
    employee_id: `STRESS${userIndex + 1}`,
    role: 'employee',
    exp: Math.floor(Date.now() / 1000) + 3600,
  }), 'rawurl');
  const signature = crypto.hmac('sha256', JWT_SECRET, `${header}.${payload}`, 'base64rawurl');
  return `${header}.${payload}.${signature}`;
}

export default function () {
  const iteration = exec.scenario.iterationInTest;
  const userIndex = iteration % envData.users.length;
  const payload = JSON.stringify({
    event_id: envData.event_id,
    ticket_type_id: envData.ticket_type_id,
    quantity: 1,
    idempotency_key: `arrival-${envData.run_id}-${iteration}-${randString(8)}`,
  });
  const params = {
    headers: {
      Authorization: `Bearer ${forgeJWT(userIndex)}`,
      'Content-Type': 'application/json',
    },
    timeout: __ENV.HTTP_TIMEOUT || '30s',
  };

  const res = http.post(`${BASE_URL}/applications`, payload, params);
  if (res.status === 200 || res.status === 201 || res.status === 202) {
    bookingSuccess.add(1);
    if (res.status === 202) bookingQueued202.add(1);
    bookingSuccessRate.add(true);
    bookingFailureRate.add(false);
  } else {
    bookingSuccessRate.add(false);
    bookingFailureRate.add(true);
    if (res.status === 409) bookingSoldOut409.add(1);
    else if (res.status === 429) bookingTooManyRequests429.add(1);
    else if (res.status >= 500 || res.status === 0) bookingServerError.add(1);
    else bookingOtherStatus.add(1);
  }

  check(res, {
    'booking arrival status is accepted or sold out': (r) => [200, 201, 202, 409].includes(r.status),
    'booking arrival status is not 5xx': (r) => r.status < 500,
  });

  sleep(Math.random() * 0.05);
}
