
import http from 'k6/http';
import { check, sleep } from 'k6';
import { vu } from 'k6/execution';
import crypto from 'k6/crypto';
import encoding from 'k6/encoding';
import { SharedArray } from 'k6/data';
import { Counter, Rate } from 'k6/metrics';

const envData = new SharedArray('stress env', function () {
  return [JSON.parse(open('./stress_env.json'))];
})[0];

const TEST_MODE = __ENV.TEST_MODE || 'spike';
const RATE = Number(__ENV.RATE || 1000);
const PRE_ALLOCATED_VUS = Number(__ENV.PRE_ALLOCATED_VUS || 500);
const MAX_VUS = Number(__ENV.MAX_VUS || 2000);

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8001/v1';
const VUS = Number(__ENV.VUS || 1000);
const ITERATIONS = Number(__ENV.ITERATIONS || 1);
const MAX_DURATION = __ENV.MAX_DURATION || '3m';
const STRICT_THRESHOLDS = (__ENV.STRICT_THRESHOLDS || '0') === '1';
const ERROR_SAMPLE_RATE = Number(__ENV.ERROR_SAMPLE_RATE || 0.02);
const MAX_ERROR_BODY_LENGTH = Number(__ENV.MAX_ERROR_BODY_LENGTH || 500);
const JWT_SECRET = __ENV.JWT_SECRET || 'dev-jwt-secret-change-in-prod-32chars!!';
const DISCARD_RESPONSE_BODIES = (__ENV.DISCARD_RESPONSE_BODIES || '0') === '1';

if (!Number.isInteger(VUS) || VUS <= 0) throw new Error(`VUS must be positive integer, got ${__ENV.VUS}`);
if (!Number.isInteger(ITERATIONS) || ITERATIONS <= 0) throw new Error(`ITERATIONS must be positive integer, got ${__ENV.ITERATIONS}`);
if (VUS > envData.users.length) {
  throw new Error(`Invalid load test: VUS=${VUS} but only TOTAL_USERS=${envData.users.length}. Increase TOTAL_USERS or lower VUS.`);
}

const scenarioConfig = TEST_MODE === 'constant' 
  ? {
      constant_traffic: {
        executor: 'constant-arrival-rate',
        rate: RATE,
        timeUnit: '1s',
        duration: MAX_DURATION,
        preAllocatedVUs: PRE_ALLOCATED_VUS,
        maxVUs: MAX_VUS,
      }
    }
  : {
      mass_booking: {
        executor: 'per-vu-iterations',
        vus: VUS,
        iterations: ITERATIONS,
        maxDuration: MAX_DURATION,
      }
    };

export const options = {
  discardResponseBodies: DISCARD_RESPONSE_BODIES,
  scenarios: {
    mass_booking: {
      executor: 'per-vu-iterations',
      vus: VUS,
      iterations: ITERATIONS,
      maxDuration: MAX_DURATION,
    },
  },
  thresholds: STRICT_THRESHOLDS
    ? {
        http_req_failed: ['rate<0.05'],
        http_req_duration: ['p(95)<2000'],
        booking_server_error: ['count==0'],
      }
    : {},
};
const bookingSuccess = new Counter('booking_success');
const bookingCreated201 = new Counter('booking_created_201');
const bookingOK200 = new Counter('booking_ok_200');
const bookingQueued202 = new Counter('booking_queued_202');
const bookingSoldOut409 = new Counter('booking_sold_out_409');
const bookingBadRequest400 = new Counter('booking_bad_request_400');
const bookingUnauthorized401 = new Counter('booking_unauthorized_401');
const bookingForbidden403 = new Counter('booking_forbidden_403');
const bookingTooManyRequests429 = new Counter('booking_too_many_requests_429');
const bookingServerError = new Counter('booking_server_error');
const booking500 = new Counter('booking_500');
const booking502 = new Counter('booking_502');
const booking503 = new Counter('booking_503');
const booking504 = new Counter('booking_504');
const bookingOtherStatus = new Counter('booking_other_status');
const bookingErrorCode = new Counter('booking_error_code');
const bookingSuccessRate = new Rate('booking_success_rate');
const bookingFailureRate = new Rate('booking_failure_rate');

function generateRandomString(length) {
  const charset = 'abcdefghijklmnopqrstuvwxyz0123456789';
  let res = '';
  while (length--) res += charset[Math.random() * charset.length | 0];
  return res;
}

function forgeJWT(vuId, realUserId) {
  const header = encoding.b64encode(JSON.stringify({ alg: 'HS256', typ: 'JWT' }), 'rawurl');
  const payload = encoding.b64encode(JSON.stringify({
    user_id: realUserId,
    employee_id: `STRESS${vuId}`,
    role: 'employee',
    exp: Math.floor(Date.now() / 1000) + 3600,
  }), 'rawurl');
  const signature = crypto.hmac('sha256', JWT_SECRET, `${header}.${payload}`, 'base64rawurl');
  return `${header}.${payload}.${signature}`;
}

function parseErrorCode(body) {
  try {
    const parsed = JSON.parse(body || '{}');
    return parsed?.error?.code || parsed?.code || '';
  } catch (_) {
    return '';
  }
}

function maybeLogErrorSample(res, code) {
  if (res.status < 400) return;
  if (Math.random() > ERROR_SAMPLE_RATE) return;
  const body = (res.body || '').slice(0, MAX_ERROR_BODY_LENGTH).replace(/\s+/g, ' ');
  console.log(`[ERROR_SAMPLE] status=${res.status} code=${code || '-'} vu=${vu.idInTest} body=${body}`);
}

export default function () {
  const myUUID = envData.users[vu.idInTest - 1];
  if (!myUUID) {
    throw new Error(`No user_id for VU ${vu.idInTest}. TOTAL_USERS=${envData.users.length}, VUS=${VUS}`);
  }

  const payload = JSON.stringify({
    event_id: envData.event_id,
    ticket_type_id: envData.ticket_type_id,
    quantity: 1,
    idempotency_key: `run-${envData.run_id}-vu-${vu.idInTest}-iter-${generateRandomString(10)}`,
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${forgeJWT(vu.idInTest, myUUID)}`,
    },
    timeout: __ENV.HTTP_TIMEOUT || '30s',
  };

  const res = http.post(`${BASE_URL}/applications`, payload, params);
  const code = parseErrorCode(res.body);

  if (res.status === 201) {
    bookingCreated201.add(1);
    bookingSuccess.add(1);
    bookingSuccessRate.add(true);
    bookingFailureRate.add(false);
  } else if (res.status === 202) {
    bookingQueued202.add(1);
    bookingSuccess.add(1);
    bookingSuccessRate.add(true);
    bookingFailureRate.add(false);
  } else if (res.status === 200) {
    bookingOK200.add(1);
    bookingSuccess.add(1);
    bookingSuccessRate.add(true);
    bookingFailureRate.add(false);
  } else {
    bookingSuccessRate.add(false);
    bookingFailureRate.add(true);
    bookingErrorCode.add(1, { status: String(res.status), code: code || 'NO_CODE' });
    if (res.status === 400) bookingBadRequest400.add(1);
    else if (res.status === 401) bookingUnauthorized401.add(1);
    else if (res.status === 403) bookingForbidden403.add(1);
    else if (res.status === 409) bookingSoldOut409.add(1);
    else if (res.status === 429) bookingTooManyRequests429.add(1);
    else if (res.status === 500) { booking500.add(1); bookingServerError.add(1); }
    else if (res.status === 502) { booking502.add(1); bookingServerError.add(1); }
    else if (res.status === 503) { booking503.add(1); bookingServerError.add(1); }
    else if (res.status === 504) { booking504.add(1); bookingServerError.add(1); }
    else if (res.status >= 500) bookingServerError.add(1);
    else bookingOtherStatus.add(1);
    maybeLogErrorSample(res, code);
  }

  check(res, {
    'status is 200/201/202 (成功或已排隊)': (r) => r.status === 200 || r.status === 201 || r.status === 202,
    'status is 409 (票已售罄)': (r) => r.status === 409,
    'status is 400/403 (業務規則擋下)': (r) => r.status === 400 || r.status === 403,
    'status is not 5xx': (r) => r.status < 500,
  });

  sleep(Math.random() * 0.1);
}
