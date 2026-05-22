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
const BOOKING_RATIO = Number(__ENV.MIXED_BOOKING_RATIO || 0.1);
const DETAIL_RATIO = Number(__ENV.READ_DETAIL_RATIO || 0.2);
const JWT_SECRET = __ENV.JWT_SECRET || 'dev-jwt-secret-change-in-prod-32chars!!';

// In mixed tests, booking is deterministic instead of random.
// This avoids repeatedly booking with the same fake user and creating many
// EXCEEDS_MAX_TICKETS 400 responses, which are not read-path failures.
const BOOKING_EVERY = BOOKING_RATIO > 0 ? Math.max(1, Math.round(1 / BOOKING_RATIO)) : 0;
const DEFAULT_MAX_BOOKINGS = Math.min(
  Number(envData.total_quota || envData.users.length),
  envData.users.length,
);
const MIXED_MAX_BOOKINGS = Number(__ENV.MIXED_MAX_BOOKINGS || DEFAULT_MAX_BOOKINGS);

export const options = {
  discardResponseBodies: DISCARD_RESPONSE_BODIES,
  scenarios: {
    mixed_user_journey: {
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
        http_req_failed: ['rate<0.03'],
        http_req_duration: ['p(95)<2000'],
        mixed_server_error: ['count==0'],
      }
    : {},
};

const mixedReadOK = new Counter('mixed_read_ok');
const mixedDetailOK = new Counter('mixed_detail_ok');
const mixedBookingAccepted = new Counter('mixed_booking_accepted');
const mixedBookingCreated200201 = new Counter('mixed_booking_created_200_201');
const mixedBookingQueued202 = new Counter('mixed_booking_queued_202');
const mixedSoldOut409 = new Counter('mixed_sold_out_409');
const mixedBusinessReject400 = new Counter('mixed_business_reject_400');
const mixedServerError = new Counter('mixed_server_error');
const mixedOtherError = new Counter('mixed_other_error');
const mixedSuccessRate = new Rate('mixed_success_rate');

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

function recordRead(res, okCounter) {
  if (res.status === 200) {
    okCounter.add(1);
    mixedSuccessRate.add(true);
    return true;
  }
  mixedSuccessRate.add(false);
  if (res.status >= 500 || res.status === 0) mixedServerError.add(1);
  else mixedOtherError.add(1);
  return false;
}

function recordBooking(res) {
  if (res.status === 200 || res.status === 201 || res.status === 202) {
    mixedBookingAccepted.add(1);
    if (res.status === 202) mixedBookingQueued202.add(1);
    else mixedBookingCreated200201.add(1);
    mixedSuccessRate.add(true);
    return true;
  }
  if (res.status === 409) {
    mixedSoldOut409.add(1);
    // Sold-out is an expected business outcome in oversubscription tests.
    mixedSuccessRate.add(true);
    return false;
  }
  if (res.status === 400) {
    mixedBusinessReject400.add(1);
    // Examples: user already reached max tickets. Track separately from server errors.
    mixedSuccessRate.add(false);
    return false;
  }
  mixedSuccessRate.add(false);
  if (res.status >= 500 || res.status === 0) mixedServerError.add(1);
  else mixedOtherError.add(1);
  return false;
}

export default function () {
  const iteration = exec.scenario.iterationInTest;
  const userIndex = iteration % envData.users.length;
  const params = {
    headers: {
      Authorization: `Bearer ${forgeJWT(userIndex)}`,
      'Content-Type': 'application/json',
    },
    timeout: __ENV.HTTP_TIMEOUT || '30s',
  };

  const listRes = http.get(`${BASE_URL}/events?status=published`, params);
  recordRead(listRes, mixedReadOK);

  if (Math.random() < DETAIL_RATIO) {
    const detailRes = http.get(`${BASE_URL}/events/${envData.event_id}`, params);
    recordRead(detailRes, mixedDetailOK);
  }

  if (BOOKING_EVERY > 0 && iteration % BOOKING_EVERY === 0) {
    const bookingOrdinal = Math.floor(iteration / BOOKING_EVERY);

    // Keep mixed-journey as a realistic read-heavy test by default:
    // stop booking once all test users/quota for this run have been exercised.
    // Use MIXED_MAX_BOOKINGS to intentionally oversubscribe this script.
    if (bookingOrdinal < MIXED_MAX_BOOKINGS) {
      const bookingUserIndex = bookingOrdinal % envData.users.length;
      const bookingParams = {
        headers: {
          Authorization: `Bearer ${forgeJWT(bookingUserIndex)}`,
          'Content-Type': 'application/json',
        },
        timeout: __ENV.HTTP_TIMEOUT || '30s',
      };
      const payload = JSON.stringify({
        event_id: envData.event_id,
        ticket_type_id: envData.ticket_type_id,
        quantity: 1,
        idempotency_key: `mixed-${envData.run_id}-${bookingOrdinal}-${randString(8)}`,
      });
      const bookingRes = http.post(`${BASE_URL}/applications`, payload, bookingParams);
      recordBooking(bookingRes);
    }
  }

  check(listRes, {
    'mixed GET /events status is 200': (r) => r.status === 200,
    'mixed GET /events is not 5xx': (r) => r.status < 500,
  });

  sleep(Math.random() * 0.05);
}
