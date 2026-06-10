const fs = require('fs');
const crypto = require('crypto');

function intEnv(name, fallback) {
  const raw = process.env[name];
  if (raw === undefined || raw === '') return fallback;
  const n = Number(raw);
  if (!Number.isInteger(n) || n <= 0) {
    throw new Error(`${name} must be a positive integer, got: ${raw}`);
  }
  return n;
}

function safeSqlString(value) {
  return String(value).replace(/'/g, "''");
}

function uuidv4() {
  return crypto.randomUUID();
}

const totalUsers = intEnv('TOTAL_USERS', 2000);
const totalQuota = intEnv('TOTAL_QUOTA', 50000);
const maxTicketsPerPerson = intEnv('MAX_TICKETS_PER_PERSON', 1);
const runId = process.env.RUN_ID || `${Date.now()}`;
const eventTitle = process.env.EVENT_TITLE || `Stress Event ${runId}`;

const eventId = uuidv4();
const ticketTypeId = uuidv4();
const managerId = uuidv4();
const userIds = [];
const fakeHash = '$2a$10$fakehashstringthatislongenoughforbcrypt1234567890123';

let sql = `-- Stress test data: run_id=${safeSqlString(runId)}\n`;

sql += `INSERT INTO users (id, employee_id, name, email, department, region, role, password_hash, created_at, updated_at) VALUES ('${managerId}', 'STRESS_MGR_${safeSqlString(runId)}', 'Stress Manager', 'stress_mgr_${safeSqlString(runId)}@company.com', 'Stress Department', 'Stress Region', 'event_manager', '${fakeHash}', NOW(), NOW());\n`;

sql += `INSERT INTO events (id, title, venue, max_tickets_per_person, status, start_time, end_time, apply_deadline, publish_time, created_by, created_at, updated_at) VALUES ('${eventId}', '${safeSqlString(eventTitle)}', 'Stress Venue', ${maxTicketsPerPerson}, 'published', NOW() + INTERVAL '45 days', NOW() + INTERVAL '46 days', NOW() + INTERVAL '30 days', NOW(), '${managerId}', NOW(), NOW());\n`;

sql += `INSERT INTO ticket_types (id, event_id, name, total_quota, remaining, created_at) VALUES ('${ticketTypeId}', '${eventId}', 'Stress Ticket ${safeSqlString(runId)}', ${totalQuota}, ${totalQuota}, NOW());\n`;

for (let i = 1; i <= totalUsers; i++) {
  const uid = uuidv4();
  userIds.push(uid);
  const email = `stress_${runId}_${i}@company.com`;
  sql += `INSERT INTO users (id, employee_id, name, email, department, region, role, password_hash, created_at, updated_at) VALUES ('${uid}', 'STRESS_${safeSqlString(runId)}_${i}', 'Stress User ${i}', '${email}', 'Stress Department', 'Stress Region', 'employee', '${fakeHash}', NOW(), NOW());\n`;
}

fs.writeFileSync('load-test/setup_db.sql', sql);
fs.writeFileSync('load-test/stress_env.json', JSON.stringify({
  run_id: runId,
  event_id: eventId,
  event_title: eventTitle,
  ticket_type_id: ticketTypeId,
  total_users: totalUsers,
  total_quota: totalQuota,
  max_tickets_per_person: maxTicketsPerPerson,
  users: userIds,
}, null, 2));

console.log(`Generated load-test data: run_id=${runId}, users=${totalUsers}, quota=${totalQuota}, event=${eventId}, ticket_type=${ticketTypeId}`);
