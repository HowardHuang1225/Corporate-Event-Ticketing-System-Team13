const fs = require('fs');
const path = require('path');

const resultDir = process.argv[2] || 'load-test/results/latest';
const summaryPath = path.join(resultDir, 'book-summary.json');
const dbPath = path.join(resultDir, 'db-summary.txt');
const envPath = path.join(resultDir, 'stress_env.json');
const outPath = path.join(resultDir, 'run-summary.md');

function metric(data, name, field = 'count', fallback = 0) {
  return data.metrics?.[name]?.[field] ?? fallback;
}

function rate(data, name) {
  const v = data.metrics?.[name]?.value;
  return typeof v === 'number' ? `${(v * 100).toFixed(2)}%` : 'N/A';
}

function num(data, name, field = 'count') {
  const v = metric(data, name, field, 0);
  return typeof v === 'number' ? v : 0;
}

function p(data, name, percentile) {
  const v = data.metrics?.[name]?.[percentile];
  return typeof v === 'number' ? `${v.toFixed(2)} ms` : 'N/A';
}

function parseDb(text) {
  const out = {};
  for (const line of text.split(/\r?\n/)) {
    const idx = line.indexOf('=');
    if (idx > 0) out[line.slice(0, idx)] = line.slice(idx + 1);
  }
  return out;
}

const summary = fs.existsSync(summaryPath) ? JSON.parse(fs.readFileSync(summaryPath, 'utf8')) : { metrics: {} };
const dbText = fs.existsSync(dbPath) ? fs.readFileSync(dbPath, 'utf8') : '';
const db = parseDb(dbText);
const env = fs.existsSync(envPath) ? JSON.parse(fs.readFileSync(envPath, 'utf8')) : {};

const success = num(summary, 'booking_success');
const serverErrors = num(summary, 'booking_server_error');
const soldOut = num(summary, 'booking_sold_out_409');
const bad400 = num(summary, 'booking_bad_request_400');
const unauth401 = num(summary, 'booking_unauthorized_401');
const forbid403 = num(summary, 'booking_forbidden_403');
const tooMany429 = num(summary, 'booking_too_many_requests_429');
const other = num(summary, 'booking_other_status');
const reqs = num(summary, 'http_reqs');
const dbConsumed = Number(db.ticket_type_consumed_by_db ?? NaN);
const dbRemaining = Number(db.ticket_type_remaining ?? NaN);
const dbQuota = Number(db.ticket_type_total_quota ?? NaN);
const appCount = Number(db.applications_count ?? NaN);
const ticketCount = Number(db.tickets_count ?? NaN);
const redisInv = Number(db.redis_inventory ?? NaN);
const inventoryEquationOK = Number.isFinite(dbConsumed) && Number.isFinite(dbRemaining) && Number.isFinite(dbQuota) && dbConsumed + dbRemaining === dbQuota;
const successMatchesDb = Number.isFinite(appCount) && Number.isFinite(ticketCount) && appCount === ticketCount && appCount === success;

const md = `# Load Test Run Summary

## Run identity

| Item | Value |
|---|---|
| RUN_ID | ${env.run_id ?? process.env.RUN_ID ?? 'N/A'} |
| Event ID | ${env.event_id ?? db.event_id ?? 'N/A'} |
| Event title | ${env.event_title ?? db.event_title ?? 'N/A'} |
| Ticket type ID | ${env.ticket_type_id ?? db.ticket_type_id ?? 'N/A'} |

## Parameters

| Item | Value |
|---|---:|
| TOTAL_USERS | ${process.env.TOTAL_USERS ?? env.total_users ?? 'N/A'} |
| TOTAL_QUOTA | ${process.env.TOTAL_QUOTA ?? env.total_quota ?? 'N/A'} |
| MAX_TICKETS_PER_PERSON | ${process.env.MAX_TICKETS_PER_PERSON ?? env.max_tickets_per_person ?? 'N/A'} |
| VUS | ${process.env.VUS ?? 'N/A'} |
| ITERATIONS per VU | ${process.env.ITERATIONS ?? 'N/A'} |
| MAX_DURATION | ${process.env.MAX_DURATION ?? 'N/A'} |
| BASE_URL | ${process.env.BASE_URL ?? 'http://localhost:8001/v1'} |

## k6 business result

| Metric | Value |
|---|---:|
| total HTTP requests | ${reqs} |
| booking_success | ${success} |
| booking_sold_out_409 | ${soldOut} |
| booking_bad_request_400 | ${bad400} |
| booking_unauthorized_401 | ${unauth401} |
| booking_forbidden_403 | ${forbid403} |
| booking_too_many_requests_429 | ${tooMany429} |
| booking_server_error | ${serverErrors} |
| booking_other_status | ${other} |
| booking_success_rate | ${rate(summary, 'booking_success_rate')} |
| http_req_failed | ${rate(summary, 'http_req_failed')} |
| http_req_duration p95 | ${p(summary, 'http_req_duration', 'p(95)')} |
| http_req_duration p99 | ${p(summary, 'http_req_duration', 'p(99)')} |
| http_reqs/sec | ${summary.metrics?.http_reqs?.rate?.toFixed?.(2) ?? 'N/A'} |

## DB / Redis verification

\`\`\`text
${dbText || 'db-summary.txt not found'}
\`\`\`

## Quick interpretation

- Inventory equation \`consumed_by_db + remaining = total_quota\`: **${inventoryEquationOK ? 'OK' : 'NEEDS_CHECK'}**.
- Applications count = tickets count = k6 success count: **${successMatchesDb ? 'OK' : 'NEEDS_CHECK'}**.
- If server errors are high while quota is not exhausted, this is likely backend/DB throughput or transaction contention, not normal sold-out behavior.
- If VUS is larger than TOTAL_USERS, the test is invalid because some VUs do not have real user IDs.
- If p95 is high and server errors are high, reduce VUS to find the maximum stable point, then compare after optimization/scale-out.
`;

fs.writeFileSync(outPath, md);
console.log(`✅ Wrote ${outPath}`);
