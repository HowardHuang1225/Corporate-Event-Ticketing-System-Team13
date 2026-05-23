import http from 'k6/http';
import { check, sleep } from 'k6';
import crypto from 'k6/crypto';
import encoding from 'k6/encoding';
import { SharedArray } from 'k6/data';
import { Counter, Rate } from 'k6/metrics';

const envData = new SharedArray('stress env', function () {
  return [JSON.parse(open('./stress_env.json'))];
})[0];

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8001/v1';
const RATE = Number(__ENV.RATE || 200);
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
    read_events: {
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
        http_req_failed: ['rate<0.01'],
        http_req_duration: ['p(95)<1000'],
        read_server_error: ['count==0'],
      }
    : {},
};

const readListOK = new Counter('read_list_ok');
const readDetailOK = new Counter('read_detail_ok');
const readServerError = new Counter('read_server_error');
const readOtherError = new Counter('read_other_error');
const readSuccessRate = new Rate('read_success_rate');

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

function recordRead(res, okCounter) {
  if (res.status === 200) {
    okCounter.add(1);
    readSuccessRate.add(true);
    return;
  }
  readSuccessRate.add(false);
  if (res.status >= 500 || res.status === 0) readServerError.add(1);
  else readOtherError.add(1);
}

export default function () {
  const userIndex = Math.floor(Math.random() * envData.users.length);
  const params = {
    headers: { Authorization: `Bearer ${forgeJWT(userIndex)}` },
    timeout: __ENV.HTTP_TIMEOUT || '30s',
  };

  const listRes = http.get(`${BASE_URL}/events?status=published`, params);
  recordRead(listRes, readListOK);

  if (Math.random() < Number(__ENV.READ_DETAIL_RATIO || 0.3)) {
    const detailRes = http.get(`${BASE_URL}/events/${envData.event_id}`, params);
    recordRead(detailRes, readDetailOK);
  }

  check(listRes, {
    'GET /events status is 200': (r) => r.status === 200,
    'GET /events is not 5xx': (r) => r.status < 500,
  });

  sleep(Math.random() * 0.05);
}
