// 引入 k6 內建的模組
import http from 'k6/http';
import { check, sleep } from 'k6';

// 1. 設定測試選項 (Options)
// 這裡我們模擬 1 個虛擬使用者 (Virtual User, VU) 持續打 10 秒鐘
export const options = {
    vus: 1,         // 虛擬使用者數量：1 個人
    duration: '10s', // 測試持續時間：10 秒
};

// 2. 核心測試邏輯 (The Default Function)
// 剛才設定的每個 VU，都會不斷重複執行這個 function 裡的動作
export default function () {
    // 第一步：發送 HTTP GET 請求到後端的 /health 網址
    // 你的後端已經在 Docker 裡跑在 8001 port 了
    const res = http.get('http://localhost:8001/health');

    // 第二步：檢查 (Check) 伺服器的回應是不是正常的
    // 正常的網頁回應狀態碼 (Status Code) 應該要是 200 OK
    check(res, {
        'is status 200 (伺服器正常回應)': (r) => r.status === 200,
    });

    // 第三步：讓這個虛擬使用者喘口氣
    // 如果不加這個，k6 會用盡全力狂打，你的電腦可能會先當機！
    // sleep(1) 代表暫停 1 秒鐘，再進入下一次迴圈
    sleep(1);
}