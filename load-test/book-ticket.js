import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '5s', target: 50 },  // 模擬 50 人湧入
        { duration: '10s', target: 50 },
        { duration: '5s', target: 0 },
    ],
};

// 產生隨機字串的輔助函數 (用來當作 idempotency_key)
function generateRandomString(length) {
    const charset = 'abcdefghijklmnopqrstuvwxyz0123456789';
    let res = '';
    while (length--) res += charset[Math.random() * charset.length | 0];
    return res;
}

export default function () {
    // 1. 對齊 YAML: servers + paths
    const url = 'http://localhost:8001/v1/applications';

    // 2. 對齊 YAML: requestBody properties
    // 注意：這裡的 UUID 需要你先進資料庫看一下，填入一組真實存在的活動與票種 ID
    const payload = JSON.stringify({
        event_id: "1421a4f6-50bc-482a-b9c1-e3e623b88e06",        // 替換成真實的活動 ID
        ticket_type_id: "28c42bb5-dcc2-4770-8e45-02289bf442c3",  // 替換成真實的票種 ID
        quantity: 1,
        idempotency_key: `req-${generateRandomString(10)}`       // 每次產生隨機的防重複金鑰
    });

    // 3. 對齊 YAML: security (BearerAuth)
    const params = {
        headers: {
            'Content-Type': 'application/json',
            // 注意：請向後端同學要一組可以用的 JWT Token 貼在這裡
            'Authorization': 'Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiNGU3OWY2N2UtNjkxNi00MTUyLTgyMjgtY2Q5M2Y1NTI5YzVmIiwiZW1wbG95ZWVfaWQiOiJFTVAwMDEiLCJyb2xlIjoiZW1wbG95ZWUiLCJleHAiOjE3NzgyMjkzNjMsImlhdCI6MTc3ODE0Mjk2MywianRpIjoiMmU2NDk5YjgtZjg5Ni00OTdhLTk4MWEtN2VkZjFlNjdmMWIxIn0.XjcDl38XMAxKudkdGvwQWALlnYTifmkR15tPrhaZAbs', 
        },
    };

    // 4. 發送 POST 請求
    const res = http.post(url, payload, params);

    // 5. 驗證結果
    check(res, {
        'status is 200/201 (成功搶到)': (r) => r.status === 200 || r.status === 201,
        'status is 409 (票已售罄)': (r) => r.status === 409,
        'status is 403 (不符資格)': (r) => r.status === 403,
        'status is 400 (違反規則/超量)': (r) => r.status === 400,
    });

    // 監視器：如果狀態碼不是我們預期的這幾種，就把伺服器說的話印出來！
    if (![200, 201, 400, 403, 409].includes(res.status)) {
        console.log(`[驚喜狀態碼 ${res.status}] 伺服器說: ${res.body}`);
    } else if (res.status === 400) {
        // 印出幾條 400 錯誤看看是不是真的因為限購
        console.log(`[被擋下了] 伺服器說: ${res.body}`);
    }

    sleep(Math.random() * 0.3);
}