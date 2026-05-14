const fs = require('fs');
const crypto = require('crypto');

// 產生標準 UUID
function uuidv4() {
    return crypto.randomUUID();
}

const eventId = uuidv4();
const ticketTypeId = uuidv4();
const userIds = [];

// 1. 準備 SQL 語法
let sql = `-- 壓測專用資料 (測試結束後可安全刪除)\n`;

// 🎯 關鍵修復 1：先建立一個「假主辦人」，並取得他的 ID
const managerId = uuidv4();
let fakeHash = '$2a$10$fakehashstringthatislongenoughforbcrypt1234567890123';
sql += `INSERT INTO users (id, employee_id, name, email, department, region, role, password_hash, created_at, updated_at) VALUES ('${managerId}', 'STRESS_MGR', '壓測主辦', 'stress_mgr@company.com', '壓測部', '雲廠', 'event_manager', '${fakeHash}', NOW(), NOW());\n`;

// 🎯 完美修復：events 加上 publish_time
sql += `INSERT INTO events (id, title, venue, max_tickets_per_person, status, start_time, end_time, apply_deadline, publish_time, created_by, created_at, updated_at) VALUES ('${eventId}', '8萬人極限壓測', '虛擬巨蛋', 100, 'published', NOW() + INTERVAL '45 days', NOW() + INTERVAL '46 days', NOW() + INTERVAL '30 days', NOW(), '${managerId}', NOW(), NOW());\n`;

// 🎯 完美修復：ticket_types 移除 updated_at
sql += `INSERT INTO ticket_types (id, event_id, name, total_quota, remaining, created_at) VALUES ('${ticketTypeId}', '${eventId}', '壓測票', 50000, 50000, NOW());\n`;
// 塞入 2000 個壓測機器人
for (let i = 1; i <= 2000; i++) {
    let uid = uuidv4();
    userIds.push(uid);
    let fakeHash = '$2a$10$fakehashstringthatislongenoughforbcrypt1234567890123';
    let email = `stress${i}@company.com`;
    let department = '壓測部';
    let region = '雲端廠';
    
    sql += `INSERT INTO users (id, employee_id, name, email, department, region, role, password_hash, created_at, updated_at) VALUES ('${uid}', 'STRESS${i}', '壓測兵${i}', '${email}', '${department}', '${region}', 'employee', '${fakeHash}', NOW(), NOW());\n`;
}

// 2. 輸出給資料庫吃的 SQL 檔
fs.writeFileSync('load-test/setup_db.sql', sql);

// 3. 輸出給 k6 吃的 JSON 檔
fs.writeFileSync('load-test/stress_env.json', JSON.stringify({
    event_id: eventId,
    ticket_type_id: ticketTypeId,
    users: userIds
}, null, 2));

console.log("✅ Finished! setup_db.sql and stress_env.json generated successfully.");