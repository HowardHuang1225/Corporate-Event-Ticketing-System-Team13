import http from 'k6/http';
import { check, sleep } from 'k6';

// 這裡就是「雲端原生」架構最在乎的壓力設定
export const options = {
    // Stages 可以讓我們模擬「人潮逐漸湧入與散去」的過程
    stages: [
        { duration: '10s', target: 50 },  // 第一階段：在 10 秒內，將在線人數從 0 快速拉升到 50 人 (模擬開賣瞬間)
        { duration: '20s', target: 50 },  // 第二階段：保持 50 人同時在線，持續狂點 20 秒 (模擬搶票高峰期)
        { duration: '10s', target: 0 },   // 第三階段：在 10 秒內，人數逐漸降回 0 人 (模擬搶完票人潮散去)
    ],
};

export default function () {
    // 雖然我們目前還是先打 health API，但這次有 50 個人同時打
    const res = http.get('http://localhost:8001/health');

    check(res, {
        'status is 200': (r) => r.status === 200,
    });

    // 稍微停頓 0.5 到 1.5 秒，模擬真實人類滑鼠點擊的隨機反應時間
    sleep(Math.random() + 0.5); 
}