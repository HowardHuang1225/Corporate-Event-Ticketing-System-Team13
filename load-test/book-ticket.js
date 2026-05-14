import http from 'k6/http';
import { check, sleep } from 'k6';
import { vu } from 'k6/execution';
import crypto from 'k6/crypto';
import encoding from 'k6/encoding';
// 1. 新增 k6 的檔案讀取套件
import { SharedArray } from 'k6/data'; 

// 2. 讓 k6 讀取我們等一下要生成的壓測資料包 (JSON)
const envData = new SharedArray('stress env', function () {
    return [JSON.parse(open('./stress_env.json'))];
})[0];

export const options = {
    scenarios: {
        mass_booking: {
            executor: 'per-vu-iterations',
            vus: 1000,          // Total number of simulated concurrent users
            iterations: 1,      // Number of booking attempts per user
            maxDuration: '3m',  // Maximum time allowed for the test
        },
    },
};

function generateRandomString(length) {
    const charset = 'abcdefghijklmnopqrstuvwxyz0123456789';
    let res = '';
    while (length--) res += charset[Math.random() * charset.length | 0];
    return res;
}

// 3. 魔法函數改版：接收真正的 UUID 當作參數
function forgeJWT(vuId, realUserId) {
    const secret = 'dev-jwt-secret-change-in-prod-32chars!!'; 
    const header = encoding.b64encode(JSON.stringify({ alg: 'HS256', typ: 'JWT' }), 'rawurl');

    const payload = encoding.b64encode(JSON.stringify({
        user_id: realUserId, // 👈 這裡變成動態塞入真實身分證字號！
        employee_id: `STRESS${vuId}`, 
        role: 'employee',
        exp: Math.floor(Date.now() / 1000) + 3600
    }), 'rawurl');

    const signature = crypto.hmac('sha256', secret, `${header}.${payload}`, 'base64rawurl');
    return `${header}.${payload}.${signature}`;
}

export default function () {
    const url = 'http://localhost:8001/v1/applications';

    // 4. payload 不再寫死！直接從 JSON 裡面抓活動 ID 和票種 ID
    const payload = JSON.stringify({
        event_id: envData.event_id,        
        ticket_type_id: envData.ticket_type_id,  
        quantity: 1,
        idempotency_key: `req-${generateRandomString(10)}`       
    });

    // 5. 從 JSON 陣列中取出這個虛擬使用者專屬的 UUID
    const myUUID = envData.users[vu.idInTest - 1];
    
    // 把專屬 UUID 餵給魔法函數，取得手環
    const myFakeToken = forgeJWT(vu.idInTest, myUUID);

    const params = {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${myFakeToken}`, 
        },
    };

    const res = http.post(url, payload, params);

    check(res, {
        'status is 200/201 (成功搶到)': (r) => r.status === 200 || r.status === 201,
        'status is 409 (票已售罄)': (r) => r.status === 409,
        'status is 403 (不符資格)': (r) => r.status === 403,
        'status is 400 (違反規則/超量)': (r) => r.status === 400,
    });
    // if (![200, 201, 400, 403, 409].includes(res.status)) {
    //     console.log(`[驚喜狀態碼 ${res.status}] 伺服器說: ${res.body}`);
    // }

    sleep(Math.random() * 0.1);
}