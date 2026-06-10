const fs = require('fs');
const path = require('path');

const resultDir = process.argv[2];
const totalShards = Number(process.argv[3] || 0);

if (!resultDir) {
  console.error('Usage: node load-test/aggregate_k6_shards.js <result_dir> [total_shards]');
  process.exit(2);
}

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function metricWeight(summary, name) {
  const metrics = summary.metrics || {};
  if (name.startsWith('iteration')) return metrics.iterations?.count || 1;
  return metrics.http_reqs?.count || metrics.iterations?.count || 1;
}

function isCounter(metric) {
  return metric && typeof metric.count === 'number';
}

function isRate(metric) {
  return metric && typeof metric.passes === 'number' && typeof metric.fails === 'number';
}

function isTrend(metric) {
  return metric && typeof metric.avg === 'number' && typeof metric.min === 'number' && typeof metric.max === 'number';
}

function isGauge(metric) {
  return metric && typeof metric.value === 'number' && typeof metric.min === 'number' && typeof metric.max === 'number';
}

function aggregateMetrics(summaries) {
  const names = new Set();
  for (const item of summaries) {
    for (const name of Object.keys(item.summary.metrics || {})) names.add(name);
  }

  const output = {};
  for (const name of [...names].sort()) {
    const values = summaries
      .map((item) => ({ item, metric: item.summary.metrics?.[name] }))
      .filter((entry) => entry.metric);

    if (!values.length) continue;

    if (values.every((entry) => isCounter(entry.metric))) {
      output[name] = {
        count: values.reduce((sum, entry) => sum + (entry.metric.count || 0), 0),
        rate: values.reduce((sum, entry) => sum + (entry.metric.rate || 0), 0),
      };
      continue;
    }

    if (values.every((entry) => isRate(entry.metric))) {
      const passes = values.reduce((sum, entry) => sum + (entry.metric.passes || 0), 0);
      const fails = values.reduce((sum, entry) => sum + (entry.metric.fails || 0), 0);
      output[name] = {
        passes,
        fails,
        value: passes + fails > 0 ? passes / (passes + fails) : 0,
      };
      continue;
    }

    if (values.every((entry) => isTrend(entry.metric))) {
      let weightSum = 0;
      let avgSum = 0;
      let medSum = 0;
      let p90Max = 0;
      let p95Max = 0;
      for (const entry of values) {
        const weight = metricWeight(entry.item.summary, name);
        weightSum += weight;
        avgSum += entry.metric.avg * weight;
        medSum += (entry.metric.med || 0) * weight;
        p90Max = Math.max(p90Max, entry.metric['p(90)'] || 0);
        p95Max = Math.max(p95Max, entry.metric['p(95)'] || 0);
      }
      output[name] = {
        avg: weightSum > 0 ? avgSum / weightSum : 0,
        min: Math.min(...values.map((entry) => entry.metric.min)),
        med: weightSum > 0 ? medSum / weightSum : 0,
        max: Math.max(...values.map((entry) => entry.metric.max)),
        'p(90)': p90Max,
        'p(95)': p95Max,
        aggregation: 'avg/med are weighted by request count; p(90)/p(95) are max shard quantiles',
      };
      continue;
    }

    if (values.every((entry) => isGauge(entry.metric))) {
      output[name] = {
        value: values.reduce((sum, entry) => sum + (entry.metric.value || 0), 0),
        min: Math.min(...values.map((entry) => entry.metric.min)),
        max: values.reduce((sum, entry) => sum + (entry.metric.max || 0), 0),
      };
    }
  }

  return output;
}

function aggregateChecks(summaries) {
  const checks = {};
  for (const item of summaries) {
    const sourceChecks = item.summary.root_group?.checks || {};
    for (const [name, check] of Object.entries(sourceChecks)) {
      if (!checks[name]) {
        checks[name] = {
          id: check.id,
          name: check.name || name,
          path: check.path,
          passes: 0,
          fails: 0,
        };
      }
      checks[name].passes += check.passes || 0;
      checks[name].fails += check.fails || 0;
    }
  }
  return checks;
}

const summaries = [];
const missing = [];
const shardCount = totalShards > 0
  ? totalShards
  : fs.readdirSync(path.join(resultDir, 'shards')).filter((name) => /^shard\d+$/.test(name)).length;

for (let i = 1; i <= shardCount; i++) {
  const shardDir = path.join(resultDir, 'shards', `shard${i}`);
  const summaryPath = path.join(shardDir, 'book-summary.json');
  if (!fs.existsSync(summaryPath)) {
    missing.push(i);
    continue;
  }
  const summary = readJSON(summaryPath);
  const endpointPath = path.join(shardDir, 'endpoint.txt');
  const endpoint = fs.existsSync(endpointPath) ? fs.readFileSync(endpointPath, 'utf8').trim() : '';
  summaries.push({ shard: i, endpoint, summary });
}

const metrics = aggregateMetrics(summaries);
const checks = aggregateChecks(summaries);
const shardStats = summaries.map((item) => {
  const metrics = item.summary.metrics || {};
  const reqs = metrics.http_reqs?.count || 0;
  const success = metrics.booking_success?.count || 0;
  return {
    shard: item.shard,
    endpoint: item.endpoint,
    http_reqs: reqs,
    booking_success: success,
    booking_queued_202: metrics.booking_queued_202?.count || 0,
    failures: reqs - success,
    http_req_failed_value: metrics.http_req_failed?.value ?? null,
    booking_server_error: metrics.booking_server_error?.count || 0,
    http_req_duration_p95_ms: metrics.http_req_duration?.['p(95)'] ?? null,
  };
});

const totalReqs = metrics.http_reqs?.count || 0;
const totalSuccess = metrics.booking_success?.count || 0;
const totalQueued = metrics.booking_queued_202?.count || 0;
const serverErrors = metrics.booking_server_error?.count || 0;
const successRate = totalReqs > 0 ? (totalSuccess / totalReqs) * 100 : 0;

const aggregateSummary = {
  aggregation: {
    type: 'k6-shard-summary',
    source_shards: summaries.length,
    missing_shards: missing,
    note: 'Trend quantiles are aggregated as max shard quantiles because raw samples are not available in k6 summary exports.',
  },
  root_group: {
    name: '',
    path: '',
    id: 'aggregate',
    groups: {},
    checks,
  },
  metrics,
  totals: {
    total_reqs: totalReqs,
    total_success: totalSuccess,
    total_queued: totalQueued,
    total_failures: totalReqs - totalSuccess,
    success_rate_percent: Number(successRate.toFixed(4)),
    server_errors: serverErrors,
    max_shard_p95_ms: metrics.http_req_duration?.['p(95)'] || 0,
  },
  shards: shardStats,
};

fs.writeFileSync(path.join(resultDir, 'book-summary.json'), JSON.stringify(aggregateSummary, null, 2));

console.log(`total_reqs=${totalReqs}`);
console.log(`total_success=${totalSuccess}`);
console.log(`total_queued=${totalQueued}`);
console.log(`total_failures=${totalReqs - totalSuccess}`);
console.log(`success_rate_percent=${successRate.toFixed(4)}`);
console.log(`server_errors=${serverErrors}`);
console.log(`max_shard_p95_ms=${metrics.http_req_duration?.['p(95)'] || 0}`);
for (const shard of shardStats) {
  console.log(`shard${shard.shard} endpoint=${shard.endpoint} reqs=${shard.http_reqs} success=${shard.booking_success} queued=${shard.booking_queued_202} failure=${shard.failures} p95_ms=${shard.http_req_duration_p95_ms}`);
}
for (const shard of missing) {
  console.log(`shard${shard} summary_missing`);
}
